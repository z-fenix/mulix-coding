package preset

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallResult reports what was installed, for the CLI to print.
type InstallResult struct {
	ID      string
	Version string
	Dir     string
}

// InstallFromDir installs the preset found at srcDir (which must contain a
// preset.yml at its root) by copying it into .mulix/presets/<id>/ under
// root and recording it in the registry. Re-installing the same id
// overwrites its files and registry entry (version/source/hash), but
// leaves priority/enabled alone if the entry already existed, so a
// re-install doesn't silently reset a user's ordering choice.
func InstallFromDir(root, srcDir string) (InstallResult, error) {
	manifestPath := filepath.Join(srcDir, "preset.yml")
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return InstallResult{}, err
	}

	dstDir := InstallDir(root, m.Preset.ID)
	if err := copyDir(srcDir, dstDir); err != nil {
		return InstallResult{}, fmt.Errorf("preset: copying %s to %s: %w", srcDir, dstDir, err)
	}

	hash, err := manifestHash(filepath.Join(dstDir, "preset.yml"))
	if err != nil {
		return InstallResult{}, err
	}

	if err := recordInstall(root, m.Preset.ID, m.Preset.Version, srcDir, hash); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{ID: m.Preset.ID, Version: m.Preset.Version, Dir: dstDir}, nil
}

// InstallFromURL downloads a zip archive from rawURL (must be HTTPS),
// optionally verifying it against sha256Hex, and installs whichever
// preset.yml is found at the archive's shallowest level (GitHub's
// "Source code (zip)" archives wrap everything in one top-level
// directory, so this walks past that wrapper the same way spec-kit's
// installer tolerates it).
func InstallFromURL(root, rawURL, sha256Hex string) (InstallResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return InstallResult{}, fmt.Errorf("preset: invalid URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLocalhost(parsed.Hostname())) {
		return InstallResult{}, fmt.Errorf("preset: download URL must use HTTPS: %s", rawURL)
	}

	data, err := downloadAll(rawURL)
	if err != nil {
		return InstallResult{}, err
	}

	if sha256Hex != "" {
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if !strings.EqualFold(got, sha256Hex) {
			return InstallResult{}, fmt.Errorf("preset: sha256 mismatch for %s: expected %s, got %s", rawURL, sha256Hex, got)
		}
	}

	tmpDir, err := os.MkdirTemp("", "mulix-preset-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("preset: creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZip(data, tmpDir); err != nil {
		return InstallResult{}, err
	}

	srcDir, err := findManifestDir(tmpDir)
	if err != nil {
		return InstallResult{}, err
	}

	m, err := LoadManifest(filepath.Join(srcDir, "preset.yml"))
	if err != nil {
		return InstallResult{}, err
	}

	dstDir := InstallDir(root, m.Preset.ID)
	if err := copyDir(srcDir, dstDir); err != nil {
		return InstallResult{}, fmt.Errorf("preset: copying extracted preset to %s: %w", dstDir, err)
	}

	hash, err := manifestHash(filepath.Join(dstDir, "preset.yml"))
	if err != nil {
		return InstallResult{}, err
	}

	if err := recordInstall(root, m.Preset.ID, m.Preset.Version, rawURL, hash); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{ID: m.Preset.ID, Version: m.Preset.Version, Dir: dstDir}, nil
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// recordInstall adds/updates id's registry entry, preserving an existing
// entry's Priority/Enabled across a re-install (only Version/Source/Hash/
// InstalledAt are refreshed), so `preset add` twice doesn't reset ordering
// a user already set with `preset set-priority`.
func recordInstall(root, id, version, source, hash string) error {
	reg, err := LoadRegistry(root)
	if err != nil {
		return err
	}
	entry, existed := reg.Presets[id]
	if !existed {
		entry = Entry{Priority: DefaultPriority, Enabled: true}
	}
	entry.Version = version
	entry.Source = source
	entry.ManifestHash = hash
	entry.InstalledAt = time.Now().UTC()
	reg.Presets[id] = entry
	return Save(root, reg)
}

func manifestHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("preset: reading %s for hashing: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func downloadAll(rawURL string) ([]byte, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("preset: downloading %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("preset: downloading %s: unexpected status %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("preset: reading response body from %s: %w", rawURL, err)
	}
	return data, nil
}

// extractZip writes every file in the archive data into destDir,
// rejecting entries that would escape destDir (zip-slip guard).
func extractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("preset: reading zip archive: %w", err)
	}
	for _, f := range r.File {
		cleaned := filepath.Clean(f.Name)
		if cleaned == "." || strings.HasPrefix(cleaned, "..") {
			continue
		}
		target := filepath.Join(destDir, cleaned)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("preset: zip entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("preset: creating %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("preset: creating %s: %w", filepath.Dir(target), err)
		}
		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("preset: opening zip entry %q: %w", f.Name, err)
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			src.Close()
			return fmt.Errorf("preset: writing %s: %w", target, err)
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			return fmt.Errorf("preset: writing %s: %w", target, copyErr)
		}
	}
	return nil
}

// findManifestDir searches root and its immediate subdirectories for a
// preset.yml, tolerating GitHub-style zip archives that wrap everything in
// one top-level "<repo>-<ref>/" directory.
func findManifestDir(root string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, "preset.yml")); err == nil {
		return root, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("preset: reading %s: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(candidate, "preset.yml")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("preset: no preset.yml found in archive")
}

// copyDir recursively copies srcDir's contents into dstDir, replacing
// dstDir's existing contents entirely so a re-install can't leave stale
// files from a previous version behind.
func copyDir(srcDir, dstDir string) error {
	if err := os.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("removing existing %s: %w", dstDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dstDir, err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcDir, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(srcDir, e.Name())
		dstPath := filepath.Join(dstDir, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", dstPath, err)
		}
	}
	return nil
}
