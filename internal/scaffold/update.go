package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

// UpdateResult reports completed per-file operations and preserved problems.
type UpdateResult struct {
	Written    []string
	Updated    []string
	Merged     []string
	Conflicted []string
	Removed    []string
	Skipped    []string
	Accepted   []string
	Migrated   []string
	Blocked    []string
	Backups    []string
	// Partial lists completed mutations whose later per-file stage failed.
	Partial []string
}

func (r UpdateResult) NeedsAttention() bool {
	return len(r.Conflicted) > 0 || len(r.Blocked) > 0 || len(r.Partial) > 0
}

// UpdateOptions controls resource updates, not updates to the CLI binary.
type UpdateOptions struct {
	Root string
	// Force discards managed local edits only after protective backups succeed.
	Force bool
	// Accept lists manually resolved pending files; paths are project-relative.
	Accept []string
}

type updateRun struct {
	opts      UpdateOptions
	result    UpdateResult
	state     *installState
	backups   *updateBackups
	write     func(string, []byte, fs.FileMode) error
	remove    func(string) error
	saveState func(*installState) error
}

func newUpdateRun(opts UpdateOptions) (*updateRun, error) {
	if opts.Force && len(opts.Accept) > 0 {
		return nil, fmt.Errorf("scaffold: --force and --accept are mutually exclusive")
	}
	seen := map[string]bool{}
	for _, rel := range opts.Accept {
		if !validManagedRel(rel) || seen[managedPathIdentity(rel)] {
			return nil, fmt.Errorf("scaffold: invalid or duplicate --accept path %q", rel)
		}
		seen[managedPathIdentity(rel)] = true
	}
	st, err := loadInstallState(opts.Root)
	if err != nil {
		return nil, err
	}
	for _, rel := range opts.Accept {
		if _, ok := st.Pending[managedMapKey(st.Pending, rel)]; !ok {
			return nil, fmt.Errorf("scaffold: %s has no pending update", rel)
		}
	}
	return &updateRun{
		opts: opts, state: st, backups: &updateBackups{root: opts.Root},
		write: fileutil.WriteFile, remove: os.Remove,
		saveState: func(st *installState) error { return saveInstallState(opts.Root, st) },
	}, nil
}

// Update refreshes only installed hosts and managed resources. Baselines hold
// pure vendor bytes; locally edited or unknown obsolete content is preserved.
// It does not update the constitution, runtime, presets, or Claude settings.
// Callers must inspect both the partial result and error/NeedsAttention.
func Update(opts UpdateOptions) (result UpdateResult, err error) {
	run, err := newUpdateRun(opts)
	if err != nil {
		return result, err
	}
	defer func() {
		result = run.result
		if run.backups.dir != "" {
			result.Backups = []string{filepath.Join(opts.Root, filepath.FromSlash(run.backups.dir))}
		}
	}()
	hosts, err := updateHosts(opts.Root, run.state)
	if err != nil {
		return result, err
	}
	run.state.Hosts = hosts
	files, err := managedFiles(hosts)
	if err != nil {
		return result, err
	}
	byRel := make(map[string]managedFile, len(files))
	for _, f := range files {
		byRel[f.Rel] = f
	}
	for _, rel := range opts.Accept {
		f, ok := byRel[managedMapKey(byRel, rel)]
		if !ok {
			return result, fmt.Errorf("scaffold: pending %s is not shipped by this binary", rel)
		}
		if err := run.validateAcceptance(rel, f); err != nil {
			run.blockEntry(rel, err)
			return result, err
		}
	}
	accepted := map[string]bool{}
	for _, rel := range opts.Accept {
		if err := run.acceptPending(rel, byRel[managedMapKey(byRel, rel)]); err != nil {
			return result, err
		}
		accepted[rel] = true
	}
	missingPending := map[string]bool{}
	var pendingPaths []string
	for rel := range run.state.Pending {
		if _, shipped := byRel[managedMapKey(byRel, rel)]; !shipped {
			pendingPaths = append(pendingPaths, rel)
		}
	}
	sort.Strings(pendingPaths)
	for _, rel := range pendingPaths {
		_, hasBase, err := readInstalledFile(opts.Root, ".mulix/.installed/"+rel)
		if err != nil {
			return result, err
		}
		if hasBase || opts.Force {
			if err := run.removeObsolete(rel); err != nil {
				return result, err
			}
		} else {
			run.result.Blocked = append(run.result.Blocked, rel+" (pending no longer shipped, baseline missing; retained)")
		}
		missingPending[managedMapKey(missingPending, rel)] = true
	}
	handledEntries := map[string]bool{}
	readyEntries := map[Host]bool{}
	for _, host := range hosts {
		target := entryRel(host, entrySkillName)
		incoming, ok := byRel[target]
		if !ok {
			return result, fmt.Errorf("scaffold: canonical entry bundle missing for %s", host)
		}
		handled, ready, err := run.migrateEntry(host, incoming)
		if err != nil {
			return result, err
		}
		readyEntries[host] = ready
		if handled {
			handledEntries[target] = true
			for _, alias := range entryAliases {
				handledEntries[entryRel(host, alias)] = true
			}
		}
	}
	shipped := make(map[string]bool, len(files))
	for _, f := range files {
		shipped[f.Rel] = true
		if accepted[managedMapKey(accepted, f.Rel)] || handledEntries[f.Rel] {
			continue
		}
		if err := run.updateFile(f); err != nil {
			return result, err
		}
	}
	obsolete, err := unshippedBaselineFiles(opts.Root, hosts, shipped)
	if err != nil {
		return result, err
	}
	for _, rel := range obsolete {
		if missingPending[managedMapKey(missingPending, rel)] || handledEntries[managedMapKey(handledEntries, rel)] {
			continue
		}
		if err := run.removeObsolete(rel); err != nil {
			return result, err
		}
	}
	for _, host := range hosts {
		if !host.writesBootstrap() {
			continue
		}
		target := entryRel(host, entrySkillName)
		if !handledEntries[target] {
			ready, err := run.entryReady(target)
			if err != nil {
				return result, err
			}
			readyEntries[host] = ready
		}
		if !readyEntries[host] {
			run.result.Skipped = append(run.result.Skipped, agentsBootstrapFile+" (entry not ready; bootstrap retained)")
			continue
		}
		changed, err := mergeAgentsBootstrap(opts.Root, host.skillsDir())
		if err != nil {
			return result, err
		}
		if changed {
			run.result.Updated = append(run.result.Updated, agentsBootstrapFile+" (mulix section)")
		} else {
			run.result.Skipped = append(run.result.Skipped, agentsBootstrapFile+" (mulix section, already current)")
		}
	}
	if err := run.saveState(run.state); err != nil {
		return result, err
	}
	return result, nil
}

// updateHosts is symmetric: there is no implicit Claude fallback. Historical
// installations without metadata/baselines can identify only exact old entries.
func updateHosts(root string, st *installState) ([]Host, error) {
	hosts, err := installedHosts(root, st)
	if err != nil {
		return nil, err
	}
	seen := map[Host]bool{}
	for _, host := range hosts {
		seen[host] = true
	}
	for _, host := range allHosts {
		if seen[host] {
			continue
		}
		for _, name := range append([]string{entrySkillName}, entryAliases...) {
			_, exists, err := readInstalledFile(root, entryRel(host, name))
			if err != nil {
				return nil, err
			}
			if exists {
				seen[host] = true
				break
			}
		}
	}
	hosts = nil
	for _, host := range allHosts {
		if seen[host] {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("scaffold: no installed agent host found; run mulix init --host claude or --host dsh")
	}
	return hosts, nil
}

// unshippedBaselineFiles derives obsolete paths from actual baseline ownership,
// not arbitrary files in a skill directory. Errors are not missing baselines.
func unshippedBaselineFiles(root string, hosts []Host, shipped map[string]bool) ([]string, error) {
	baseProbe, err := projectFilePath(root, ".mulix/.installed/.probe")
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(baseProbe)
	owned := mulixOwnedDirs(hosts)
	var out []string
	err = filepath.WalkDir(base, func(full string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(base, full)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relative)
		if !validManagedRel(rel) || shipped[managedMapKey(shipped, rel)] {
			return nil
		}
		for _, dir := range owned {
			if strings.HasPrefix(managedPathIdentity(rel), managedPathIdentity(dir)+"/") {
				if !d.Type().IsRegular() {
					return fmt.Errorf("scaffold: baseline %s is not a regular file", rel)
				}
				out = append(out, rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scaffold: reading obsolete baselines: %w", err)
	}
	return out, nil
}

func mulixOwnedDirs(hosts []Host) []string {
	dirs := []string{SuperpowersDir, ".mulix/templates"}
	for _, host := range hosts {
		dirs = append(dirs, host.skillsDir())
	}
	return dirs
}

func (r *updateRun) removeObsolete(rel string) (err error) {
	var completed []string
	stage := "obsolete local"
	defer func() { r.recordPartial(rel, completed, stage, err) }()
	if !validManagedRel(rel) {
		return fmt.Errorf("scaffold: cannot remove unmanaged path %q", rel)
	}
	pending, hasPending := r.state.Pending[managedMapKey(r.state.Pending, rel)]
	if hasPending && (!r.opts.Force || pending.Stage != "conflict" || len(pending.Sources) > 0) {
		r.result.Blocked = append(r.result.Blocked, rel+" (obsolete with pending update; retained)")
		return nil
	}
	local, exists, err := readInstalledFile(r.opts.Root, rel)
	if err != nil {
		return err
	}
	base, hasBase, err := readInstalledFile(r.opts.Root, ".mulix/.installed/"+rel)
	if err != nil {
		return err
	}
	// A previous force may already have removed L/B before its metadata save
	// failed. Only explicit force for a registered ordinary pending may finish
	// that retirement without a baseline; normal retries remain blocked.
	if !hasBase && !(r.opts.Force && hasPending) {
		return fmt.Errorf("scaffold: obsolete baseline missing for %s", rel)
	}
	if !r.opts.Force && !r.state.trustedVendor(rel, base, nil) {
		r.result.Blocked = append(r.result.Blocked, rel+" (obsolete, no trusted baseline; retained)")
		return nil
	}
	if exists && !r.opts.Force && string(local) != string(base) {
		r.result.Blocked = append(r.result.Blocked, rel+" (obsolete, locally modified; retained)")
		return nil
	}
	if err := r.preserve(rel); err != nil {
		return err
	}
	full := filepath.Join(r.opts.Root, filepath.FromSlash(rel))
	baseFull := baselinePath(r.opts.Root, rel)
	if exists {
		if err := r.remove(full); err != nil {
			return fmt.Errorf("scaffold: removing %s: %w", rel, err)
		}
		completed = append(completed, "local removed")
	}
	stage = "obsolete baseline"
	if hasBase {
		if err := r.remove(baseFull); err != nil {
			return fmt.Errorf("scaffold: removing baseline for %s: %w", rel, err)
		}
		completed = append(completed, "vendor baseline removed")
	}
	stage = "obsolete metadata"
	old, recorded := r.state.Files[managedMapKey(r.state.Files, rel)]
	delete(r.state.Files, managedMapKey(r.state.Files, rel))
	if hasPending {
		delete(r.state.Pending, managedMapKey(r.state.Pending, rel))
	}
	if err := r.saveState(r.state); err != nil {
		if hasPending {
			r.state.Pending[managedMapKey(r.state.Pending, rel)] = pending
		}
		if recorded {
			r.state.Files[managedMapKey(r.state.Files, rel)] = old
		}
		return fmt.Errorf("scaffold: saving obsolete removal for %s: %w", rel, err)
	}
	removed := rel
	if !exists && !hasBase {
		removed += " (pending retirement completed; files already absent)"
	}
	r.result.Removed = append(r.result.Removed, removed)
	removeEmptyParents(r.opts.Root, filepath.Dir(full))
	removeEmptyParents(r.opts.Root, filepath.Dir(baseFull))
	return nil
}

func (r *updateRun) recordPartial(rel string, completed []string, stage string, err error) {
	if err != nil && len(completed) > 0 {
		r.result.Partial = append(r.result.Partial, fmt.Sprintf("%s (%s; failed: %s)", rel, strings.Join(completed, "; "), stage))
	}
}

func (r *updateRun) preserve(rel string) error {
	err := r.backups.preserve(rel)
	if r.backups.dir != "" {
		r.result.Backups = []string{filepath.Join(r.opts.Root, filepath.FromSlash(r.backups.dir))}
	}
	return err
}

// removeEmptyParents is best-effort empty-directory cleanup, never recursive.
func removeEmptyParents(root, dir string) {
	for dir = filepath.Clean(dir); dir != filepath.Clean(root); dir = filepath.Dir(dir) {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
	}
}

// baselinePath mirrors a project-relative path to its pure vendor ancestor.
func baselinePath(root, rel string) string {
	return filepath.Join(root, ".mulix", ".installed", filepath.FromSlash(rel))
}

func writeFile(full string, data []byte) error { return writeFileMode(full, data, 0o644) }
func writeFileMode(full string, data []byte, mode fs.FileMode) error {
	return fileutil.WriteFile(full, data, mode)
}

// Preserve the trailing-newline state through merge3's line representation.
func splitLines(data []byte) []string { return strings.Split(string(data), "\n") }
