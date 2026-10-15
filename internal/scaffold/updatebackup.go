package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

const auxiliaryGitignore = "*\n!.gitignore\n"

type updateBackups struct {
	root   string
	dir    string // project-relative; allocated only when preservation is needed
	backed map[string]bool
}

func (b *updateBackups) preserve(rel string) error {
	if !validManagedRel(rel) && rel != ".mulix/memory/constitution.md" && rel != ".mulix/.runtime/.gitignore" {
		return fmt.Errorf("scaffold: cannot back up unmanaged path %q", rel)
	}
	if b.backed[managedPathIdentity(rel)] {
		return nil
	}
	local, err := projectFilePath(b.root, rel)
	if err != nil {
		return err
	}
	base, err := projectFilePath(b.root, ".mulix/.installed/"+rel)
	if err != nil {
		return err
	}
	meta, err := projectFilePath(b.root, installStateRel)
	if err != nil {
		return err
	}
	if b.dir == "" {
		ignore, err := projectFilePath(b.root, ".mulix/.backups/.gitignore")
		if err != nil {
			return err
		}
		if err := ensureAuxiliaryIgnore(ignore); err != nil {
			return err
		}
		full, err := os.MkdirTemp(filepath.Dir(ignore), "update-*")
		if err != nil {
			return fmt.Errorf("scaffold: creating backup run: %w", err)
		}
		relative, err := filepath.Rel(b.root, full)
		if err != nil {
			return err
		}
		b.dir = filepath.ToSlash(relative)
		b.backed = map[string]bool{}
	}
	full := filepath.Join(b.root, filepath.FromSlash(b.dir))
	if !b.backed[installStateRel] {
		if err := copyBackupFile(meta, filepath.Join(full, "install-state.json")); err != nil {
			return err
		}
		b.backed[installStateRel] = true
	}
	for _, pair := range [][2]string{
		{local, filepath.Join(full, "local", filepath.FromSlash(rel))},
		{base, filepath.Join(full, "baseline", filepath.FromSlash(rel))},
	} {
		if err := copyBackupFile(pair[0], pair[1]); err != nil {
			return err
		}
	}
	b.backed[managedPathIdentity(rel)] = true
	return nil
}

func copyBackupFile(src, dst string) error {
	info, err := os.Lstat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("scaffold: inspecting backup source %s: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("scaffold: backup source %s is not a regular file", src)
	}
	// A partial failed preservation may already have copied this component.
	// Keep the first snapshot instead of overwriting it on retry.
	if previous, err := os.Lstat(dst); err == nil {
		if !previous.Mode().IsRegular() {
			return fmt.Errorf("scaffold: backup target %s is not a regular file", dst)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("scaffold: reading backup source %s: %w", src, err)
	}
	if err := fileutil.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("scaffold: backing up %s: %w", src, err)
	}
	return nil
}

func ensureAuxiliaryIgnore(full string) error {
	info, err := os.Lstat(full)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("scaffold: %s is not a regular file", full)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("scaffold: reading %s: %w", full, err)
		}
		if !strings.HasSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), auxiliaryGitignore) {
			return fmt.Errorf("scaffold: %s was customized; preserve its rules and add %q before retrying", full, auxiliaryGitignore)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("scaffold: inspecting %s: %w", full, err)
	}
	return fileutil.WriteFile(full, []byte(auxiliaryGitignore), 0o644)
}

// projectFilePath rejects existing symlink/non-directory parents. This is a
// preflight check, not isolation against concurrent filesystem changes.
func projectFilePath(root, rel string) (string, error) {
	if !normalizedRel(rel) {
		return "", fmt.Errorf("scaffold: invalid project path %q", rel)
	}
	current := root
	parts := strings.Split(rel, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("scaffold: inspecting parent of %s: %w", rel, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("scaffold: parent of %s is not a directory: %s", rel, current)
		}
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

func readInstalledFile(root, rel string) ([]byte, bool, error) {
	full, err := projectFilePath(root, rel)
	if err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("scaffold: inspecting %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("scaffold: %s is not a regular file", rel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, false, fmt.Errorf("scaffold: reading %s: %w", rel, err)
	}
	return data, true, nil
}

func installedHosts(root string, st *installState) ([]Host, error) {
	seen := map[Host]bool{}
	for _, host := range st.Hosts {
		seen[host] = true
	}
	for _, host := range allHosts {
		probe, err := projectFilePath(root, ".mulix/.installed/"+host.skillsDir()+"/.probe")
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(filepath.Dir(probe), func(p string, d fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("scaffold: symlink in host baseline: %s", p)
			}
			if d.Type().IsRegular() {
				seen[host] = true
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scaffold: inspecting %s baseline: %w", host, err)
		}
	}
	var hosts []Host
	for _, host := range allHosts {
		if seen[host] {
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}
