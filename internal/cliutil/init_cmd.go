package cliutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/scaffold"
	"github.com/z-fenix/mulix-coding/internal/tracker"
)

const setupBegin = "<!-- mulix setup:begin -->"
const setupEnd = "<!-- mulix setup:end -->"

func domainSetupSection(glossary string) string {
	return "# Domain docs\n\nRead the [glossary](" + glossary + ") and relevant [ADRs](../adr/) before using domain terms; proceed if absent. Do not move or replace existing domain documents."
}

func setupSections(root string, t tracker.Target) map[string]string {
	issue := "Issues and specs are local Markdown in this project. Do not assume a remote tracker."
	triageKind := "Local Markdown status"
	if t.Kind != "markdown" {
		state := "verified project ID " + t.ProjectID
		if t.ProjectID == "" {
			state = "pending verification; do not write remotely until verified and explicitly authorized"
		}
		issue = fmt.Sprintf("Issue tracker: %s at `%s`, project `%s` (%s). Pin this host and project in every command. Reading a project does not authorize remote writes.", t.Kind, t.Host, t.Project, state)
		triageKind = "Remote label"
	}
	triage := "| Role | " + triageKind + " | Meaning |\n|---|---|---|\n" +
		"| `needs-triage` | `needs-triage` | Needs maintainer evaluation |\n" +
		"| `needs-info` | `needs-info` | Waiting for more information |\n" +
		"| `ready-for-agent` | `ready-for-agent` | Specified and ready for an agent |\n" +
		"| `ready-for-human` | `ready-for-human` | Requires human implementation |\n" +
		"| `wontfix` | `wontfix` | Will not be actioned |\n\n" +
		"These are default role mappings; confirm project-specific conventions. Do not create remote labels without authorization."
	glossary := "../../GLOSSARY.md"
	if info, err := os.Stat(filepath.Join(root, "docs", "GLOSSARY.md")); err == nil && info.Mode().IsRegular() {
		glossary = "../GLOSSARY.md"
	}
	domain := domainSetupSection(glossary)
	return map[string]string{
		"docs/agents/issue-tracker.md": "# Issue tracker\n\n" + issue,
		"docs/agents/triage-labels.md": "# Triage labels\n\n" + triage,
		"docs/agents/domain.md":        domain,
		"CLAUDE.md":                    "## Mulix project guidance\n\nRead `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md`, and `docs/agents/domain.md` before using their respective conventions.",
		"AGENTS.md":                    "## Mulix project guidance\n\nRead `docs/agents/issue-tracker.md`, `docs/agents/triage-labels.md`, and `docs/agents/domain.md` before using their respective conventions.",
	}
}

// safeSetupPath rejects symlinks and non-directory ancestors beneath the selected root.
func safeSetupPath(root, rel string) (string, error) {
	current := root
	parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return "", fmt.Errorf("setup path %s is not a safe regular file path; resolve it manually", current)
		}
	}
	return current, nil
}

// prepareSetupDocs plans merges before installation, without changing user content.
func prepareSetupDocs(root string, target tracker.Target, previous tracker.Target, hasPrevious bool, hosts []scaffold.Host) (map[string][]byte, error) {
	sections := setupSections(root, target)
	old := setupSections(root, previous)
	wanted := map[string]bool{"docs/agents/issue-tracker.md": true, "docs/agents/triage-labels.md": true, "docs/agents/domain.md": true}
	for _, h := range hosts {
		if h == scaffold.HostClaude {
			wanted["CLAUDE.md"] = true
		}
		if h == scaffold.HostDSH {
			wanted["AGENTS.md"] = true
		}
	}
	result := map[string][]byte{}
	for rel := range wanted {
		full, err := safeSetupPath(root, rel)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(full)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		text := string(b)
		begin, end := strings.Index(text, setupBegin), strings.Index(text, setupEnd)
		section := setupBegin + "\n" + sections[rel] + "\n" + setupEnd
		switch {
		case begin < 0 && end < 0:
			if strings.HasPrefix(rel, "docs/agents/") && strings.TrimSpace(text) != "" {
				return nil, fmt.Errorf("%s already has project guidance; reconcile it manually before setup", rel)
			}
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			if text != "" {
				text += "\n"
			}
			text += section + "\n"
		case begin < 0 || end < begin || strings.Count(text, setupBegin) != 1 || strings.Count(text, setupEnd) != 1:
			return nil, fmt.Errorf("%s has invalid mulix setup markers; repair them manually", rel)
		default:
			current := text[begin : end+len(setupEnd)]
			normalized := strings.ReplaceAll(current, "\r\n", "\n")
			previousSection := setupBegin + "\n" + old[rel] + "\n" + setupEnd
			knownDomain := hasPrevious && rel == "docs/agents/domain.md" &&
				(normalized == setupBegin+"\n"+domainSetupSection("../../GLOSSARY.md")+"\n"+setupEnd ||
					normalized == setupBegin+"\n"+domainSetupSection("../GLOSSARY.md")+"\n"+setupEnd)
			if normalized != section && (!hasPrevious || (normalized != previousSection && !knownDomain)) {
				return nil, fmt.Errorf("%s has edited mulix setup guidance; reconcile it manually", rel)
			}
			if normalized != section {
				if strings.Contains(current, "\r\n") {
					section = strings.ReplaceAll(section, "\n", "\r\n")
				}
				text = text[:begin] + section + text[end+len(setupEnd):]
			}
		}
		result[rel] = []byte(text)
	}
	return result, nil
}

func newInitCmd() *cobra.Command {
	var force, allowPending bool
	var dir, kind, hostname, project string
	var hosts []string
	cmd := &cobra.Command{Use: "_setup", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		parsed, err := scaffold.ParseHosts(hosts)
		if err != nil {
			return err
		}
		if len(parsed) == 0 {
			return fmt.Errorf("select at least one --host (claude or dsh)")
		}
		if err := scaffold.PreflightNativeSetup(dir, parsed, force); err != nil {
			return err
		}
		target := tracker.Target{Kind: kind, Host: hostname, Project: project}
		if err := target.Validate(); err != nil {
			return err
		}
		if _, err := safeSetupPath(dir, ".mulix/tracker.json"); err != nil {
			return err
		}
		existing, exists, err := tracker.LoadConfig(dir)
		if err != nil {
			return err
		}
		if exists && target.Kind == existing.Kind && strings.EqualFold(target.Host, existing.Host) && strings.EqualFold(target.Project, existing.Project) {
			target.ProjectID = existing.ProjectID
		}
		checked, err := (tracker.Client{}).Probe(cmd.Context(), target)
		probeFailed := err != nil
		if err != nil {
			if !allowPending || !errors.Is(err, tracker.ErrUnreachable) {
				return err
			}
		} else {
			target = checked
		}
		docs, err := prepareSetupDocs(dir, target, existing, exists, parsed)
		if err != nil {
			return err
		}
		res, err := scaffold.Init(scaffold.InitOptions{Root: dir, Force: force, Hosts: parsed})
		for _, w := range res.Written {
			fmt.Printf("  wrote   %s\n", w)
		}
		for _, s := range res.Skipped {
			fmt.Printf("  skipped %s\n", s)
		}
		for _, b := range res.Backups {
			fmt.Printf("  - backup: %s\n", b)
		}
		if err != nil {
			return err
		}
		// Re-read host files after scaffold has merged its own bootstrap.
		docs, err = prepareSetupDocs(dir, target, existing, exists, parsed)
		if err != nil {
			return err
		}
		for rel, data := range docs {
			p, err := safeSetupPath(dir, rel)
			if err != nil {
				return err
			}
			mode := os.FileMode(0644)
			if info, err := os.Stat(p); err == nil {
				mode = info.Mode().Perm()
				old, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				if string(old) == string(data) {
					continue
				}
			} else if !os.IsNotExist(err) {
				return err
			}
			if err := fileutil.WriteFile(p, data, mode); err != nil {
				return err
			}
		}
		if err := tracker.SaveConfig(dir, target); err != nil {
			return err
		}
		fmt.Println("mulix setup complete. Run `mulix new \"<title>\"` to start a change.")
		for _, h := range parsed {
			if h == scaffold.HostClaude {
				fmt.Println("Claude Code: project skills and hooks installed.")
			}
			if h == scaffold.HostDSH {
				fmt.Println("DeepSeek Harness: project skills installed; follow phase discipline.")
			}
		}
		if probeFailed {
			fmt.Println("Tracker live recheck failed; any previously verified project ID was retained. No remote writes are authorized.")
		}
		if target.Kind != "markdown" && target.ProjectID == "" {
			fmt.Println("Tracker configuration is pending verification; no remote writes are authorized.")
		}
		return nil
	}}
	cmd.Flags().BoolVar(&force, "force", false, "back up and replace managed files")
	cmd.Flags().BoolVar(&allowPending, "allow-pending", false, "save an unreachable tracker as pending")
	cmd.Flags().StringVar(&dir, "dir", ".", "target project directory")
	cmd.Flags().StringSliceVar(&hosts, "host", nil, "selected agent hosts: claude,dsh")
	cmd.Flags().StringVar(&kind, "tracker", "", "tracker: markdown, github, gitlab")
	cmd.Flags().StringVar(&hostname, "hostname", "", "explicit remote hostname")
	cmd.Flags().StringVar(&project, "project", "", "explicit remote project path")
	return cmd
}
