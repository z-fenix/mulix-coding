package scaffold

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

const (
	installSchema   = "mulix.install.v1"
	installStateRel = ".mulix/install-state.json"
)

type vendorRecord struct {
	SHA256 string `json:"vendor_sha256"`
}

type pendingSource struct {
	Rel            string `json:"rel"`
	LocalSHA256    string `json:"local_sha256"`
	BaselineSHA256 string `json:"baseline_sha256,omitempty"`
}

type pendingUpdate struct {
	Stage          string          `json:"stage"`
	IncomingSHA256 string          `json:"incoming_sha256"`
	Candidate      string          `json:"candidate,omitempty"`
	LocalSHA256    string          `json:"local_sha256"`
	AcceptedSHA256 string          `json:"accepted_sha256,omitempty"`
	BackupDir      string          `json:"backup_dir,omitempty"`
	Sources        []pendingSource `json:"sources,omitempty"`
}

// Registered hashes describe pure vendor bytes, not the last merged output.
// They check consistency, not authenticity against a malicious local writer.
type installState struct {
	Schema  string                   `json:"schema"`
	Hosts   []Host                   `json:"hosts"`
	Files   map[string]vendorRecord  `json:"files"`
	Pending map[string]pendingUpdate `json:"pending,omitempty"`
}

func newInstallState() *installState {
	return &installState{
		Schema:  installSchema,
		Files:   map[string]vendorRecord{},
		Pending: map[string]pendingUpdate{},
	}
}

func contentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadInstallState(root string) (*installState, error) {
	full, err := projectFilePath(root, installStateRel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return newInstallState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("scaffold: inspecting %s: %w", installStateRel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("scaffold: %s is not a regular file", installStateRel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("scaffold: reading %s: %w", installStateRel, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	st := &installState{}
	if err := dec.Decode(st); err != nil {
		return nil, fmt.Errorf("scaffold: decoding %s: %w", installStateRel, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("scaffold: %s must contain exactly one JSON object", installStateRel)
	}
	if err := st.validate(); err != nil {
		return nil, fmt.Errorf("scaffold: invalid %s: %w", installStateRel, err)
	}
	if st.Files == nil {
		st.Files = map[string]vendorRecord{}
	}
	if st.Pending == nil {
		st.Pending = map[string]pendingUpdate{}
	}
	return st, nil
}

func saveInstallState(root string, st *installState) error {
	if err := st.validate(); err != nil {
		return fmt.Errorf("scaffold: invalid %s: %w", installStateRel, err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	full, err := projectFilePath(root, installStateRel)
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(full, data, 0o644); err != nil {
		return fmt.Errorf("scaffold: saving %s: %w", installStateRel, err)
	}
	return nil
}

func (st *installState) validate() error {
	if st.Schema != installSchema {
		return fmt.Errorf("schema %q, expected %q", st.Schema, installSchema)
	}
	seenHosts := map[Host]bool{}
	for _, host := range st.Hosts {
		if !validHost(host) || seenHosts[host] {
			return fmt.Errorf("invalid or duplicate host %q", host)
		}
		seenHosts[host] = true
	}
	seenFiles := map[string]bool{}
	for rel, record := range st.Files {
		identity := managedPathIdentity(rel)
		if !validManagedRel(rel) || !validDigest(record.SHA256) || seenFiles[identity] {
			return fmt.Errorf("invalid or duplicate vendor record for %q", rel)
		}
		seenFiles[identity] = true
	}
	seenPending := map[string]bool{}
	for rel, pending := range st.Pending {
		identity := managedPathIdentity(rel)
		if !validManagedRel(rel) || !validDigest(pending.IncomingSHA256) || !validDigest(pending.LocalSHA256) || seenPending[identity] {
			return fmt.Errorf("invalid or duplicate pending update for %q", rel)
		}
		seenPending[identity] = true
		switch pending.Stage {
		case "conflict":
			if !validAuxRel(pending.Candidate, ".mulix/.update-conflicts") {
				return fmt.Errorf("invalid conflict candidate for %q", rel)
			}
		case "accepted-cleanup":
			if len(pending.Sources) == 0 || !validDigest(pending.AcceptedSHA256) || !validAuxRel(pending.BackupDir, ".mulix/.backups") {
				return fmt.Errorf("invalid migration checkpoint for %q", rel)
			}
		default:
			return fmt.Errorf("unknown pending stage %q for %s", pending.Stage, rel)
		}
		seen := map[string]bool{}
		for _, source := range pending.Sources {
			if !validRenameSource(rel, source.Rel) || seen[managedPathIdentity(source.Rel)] || !validDigest(source.LocalSHA256) ||
				(source.BaselineSHA256 != "" && !validDigest(source.BaselineSHA256)) {
				return fmt.Errorf("invalid rename source %q for %s", source.Rel, rel)
			}
			seen[managedPathIdentity(source.Rel)] = true
		}
	}
	return nil
}

func validDigest(digest string) bool {
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func normalizedRel(rel string) bool {
	return rel != "" && path.Clean(rel) == rel && !strings.ContainsAny(rel, "\\:") &&
		!path.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Windows resource paths share an identity across case-only spellings. Keep
// original spelling for I/O and diagnostics, but never treat an alias as a
// second owned/obsolete file or a distinct metadata/pending entry.
func managedPathIdentity(rel string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(rel)
	}
	return rel
}

func managedMapKey[V any](items map[string]V, rel string) string {
	if _, ok := items[rel]; ok || runtime.GOOS != "windows" {
		return rel
	}
	identity := managedPathIdentity(rel)
	for key := range items {
		if managedPathIdentity(key) == identity {
			return key
		}
	}
	return rel
}

func validManagedRel(rel string) bool {
	if !normalizedRel(rel) {
		return false
	}
	identity := managedPathIdentity(rel)
	if identity == ".mulix/templates/overrides" || strings.HasPrefix(identity, ".mulix/templates/overrides/") {
		return false
	}
	for _, prefix := range []string{".claude/skills/", ".dsh/skills/", ".mulix/templates/", ".mulix/superpowers/"} {
		if strings.HasPrefix(identity, prefix) && len(identity) > len(prefix) {
			return true
		}
	}
	return false
}

func validAuxRel(rel, prefix string) bool {
	return normalizedRel(rel) && strings.HasPrefix(managedPathIdentity(rel), managedPathIdentity(prefix)+"/") && len(rel) > len(prefix)+1
}

func validRenameSource(target, source string) bool {
	for _, host := range allHosts {
		if managedPathIdentity(target) != managedPathIdentity(entryRel(host, entrySkillName)) {
			continue
		}
		for _, alias := range entryAliases {
			if managedPathIdentity(source) == managedPathIdentity(entryRel(host, alias)) {
				return true
			}
		}
	}
	return false
}

// incoming=nil means there is no shipped version (obsolete-file checks).
func (st *installState) trustedVendor(rel string, base, incoming []byte) bool {
	if record, ok := st.Files[managedMapKey(st.Files, rel)]; ok && record.SHA256 == contentDigest(base) {
		return true
	}
	return (incoming != nil && bytes.Equal(base, incoming)) || legacyVendorMatch(rel, base)
}
