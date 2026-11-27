// Package tracker stores the explicitly selected issue tracker identity.
package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

type Target struct {
	Kind      string `json:"kind" yaml:"kind"`
	Host      string `json:"host,omitempty" yaml:"host,omitempty"`
	Project   string `json:"project,omitempty" yaml:"project,omitempty"`
	ProjectID string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?(?::[0-9]{1,5})?$`)
var partPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// Validate rejects ambiguous, credential-bearing, and CLI-option-like targets.
func (t Target) Validate() error {
	switch t.Kind {
	case "markdown":
		if t.Host != "" || t.Project != "" || t.ProjectID != "" {
			return fmt.Errorf("markdown tracker cannot have a remote target")
		}
	case "github", "gitlab":
		if !hostPattern.MatchString(t.Host) || strings.Contains(t.Host, "..") {
			return fmt.Errorf("tracker requires an explicit valid hostname")
		}
		parts := strings.Split(t.Project, "/")
		if len(parts) < 2 || (t.Kind == "github" && len(parts) != 2) {
			return fmt.Errorf("tracker requires an explicit project path")
		}
		for _, p := range parts {
			if !partPattern.MatchString(p) || p == "." || p == ".." {
				return fmt.Errorf("invalid tracker project path")
			}
		}
		if t.ProjectID != "" && !decimalID(t.ProjectID) {
			return fmt.Errorf("invalid tracker project ID")
		}
	default:
		return fmt.Errorf("tracker must be markdown, github, or gitlab")
	}
	return nil
}

// LoadConfig distinguishes an absent file from a malformed or unsafe config.
func LoadConfig(root string) (Target, bool, error) {
	var target Target
	path, err := fileutil.ControlPath(root, ".mulix/tracker.json", false)
	if err != nil {
		return target, false, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return target, false, nil
	}
	if err != nil {
		return target, false, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&target); err != nil {
		return Target{}, true, fmt.Errorf("invalid tracker configuration: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Target{}, true, fmt.Errorf("invalid tracker configuration: trailing data")
	}
	if err := target.Validate(); err != nil {
		return Target{}, true, err
	}
	return target, true, nil
}

// SaveConfig writes only the target identity, never credentials.
func SaveConfig(root string, target Target) error {
	if err := target.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return err
	}
	path, err := fileutil.ControlPath(root, ".mulix/tracker.json", true)
	if err != nil {
		return err
	}
	return fileutil.WriteFile(path, append(b, '\n'), 0600)
}
