package cliutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/tracker"
	"gopkg.in/yaml.v3"
)

// A copy of the test executable acts as a portable fake remote CLI.
func init() {
	if os.Getenv("MULIX_SETUP_TEST_HELPER") != "1" {
		return
	}
	if os.Getenv("MULIX_SETUP_TEST_FAIL") == "1" {
		os.Exit(1)
	}
	fmt.Printf(`{"id":%s,"full_name":"Team/Repo"}`, os.Getenv("MULIX_SETUP_TEST_ID"))
	os.Exit(0)
}

func setupFakeGH(t *testing.T) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	dir := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("MULIX_SETUP_TEST_HELPER", "1")
	t.Setenv("MULIX_SETUP_TEST_ID", "42")
}

func TestSetupRetainsVerifiedIdentityOfflineAndRejectsReplacement(t *testing.T) {
	root := t.TempDir()
	setupFakeGH(t)
	args := []string{"_setup", "--dir", root, "--host", "claude", "--tracker", "github", "--hostname", "GitHub.example.test:8443", "--project", "Team/Repo"}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("initial setup: %v %s", err, out)
	}
	t.Setenv("MULIX_SETUP_TEST_FAIL", "1")
	out, err := runPresetCLI(t, append(args, "--allow-pending")...)
	if err != nil {
		t.Fatalf("offline setup: %v %s", err, out)
	}
	config, _, err := tracker.LoadConfig(root)
	if err != nil || config.ProjectID != "42" {
		t.Fatalf("offline setup lost verified identity: %+v %v", config, err)
	}
	if !strings.Contains(strings.ToLower(out), "recheck") {
		t.Fatalf("offline recheck failure not reported: %s", out)
	}
	t.Setenv("MULIX_SETUP_TEST_FAIL", "")
	t.Setenv("MULIX_SETUP_TEST_ID", "77")
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("replacement accepted after offline recheck")
	}
}

func TestSetupCaseOnlySpellingRetainsPin(t *testing.T) {
	root := t.TempDir()
	setupFakeGH(t)
	args := []string{"_setup", "--dir", root, "--host", "claude", "--tracker", "github", "--hostname", "GitHub.example.test:8443", "--project", "Team/Repo"}
	if _, err := runPresetCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULIX_SETUP_TEST_ID", "77")
	args[len(args)-3] = "github.EXAMPLE.test:8443"
	args[len(args)-1] = "team/repo"
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("case-only spelling accepted replacement project")
	}
	config, _, err := tracker.LoadConfig(root)
	if err != nil || config.ProjectID != "42" {
		t.Fatalf("original target changed: %+v %v", config, err)
	}
	args[len(args)-3] = "github.example.test:8444"
	if _, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("distinct port should allow explicitly selected new target: %v", err)
	}
	config, _, err = tracker.LoadConfig(root)
	if err != nil || config.ProjectID != "77" || config.Host != "github.example.test:8444" {
		t.Fatalf("distinct port identity wrong: %+v %v", config, err)
	}
}

func TestSetupRejectsStaleLayoutResources(t *testing.T) {
	for _, rel := range []string{".claude/skills/mulix-clarify/SKILL.md", ".claude/skills/subagent-driven-development/scripts/task-brief", ".dsh/skills/executing-plans/scripts/task-start"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			args := []string{"_setup", "--dir", root, "--host", "claude,dsh", "--tracker", "markdown"}
			if out, err := runPresetCLI(t, args...); err != nil {
				t.Fatalf("fresh setup: %v %s", err, out)
			}
			if err := os.Remove(filepath.Join(root, ".mulix/tracker.json")); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(root, rel)
			stale := []byte("stale pre-native workflow\n")
			if err := os.WriteFile(file, stale, 0644); err != nil {
				t.Fatal(err)
			}
			out, err := runPresetCLI(t, args...)
			if err == nil || !strings.Contains(fmt.Sprint(err)+out, "mulix update") {
				t.Fatalf("stale resource accepted: %v %s", err, out)
			}
			if _, err := os.Stat(filepath.Join(root, ".mulix/tracker.json")); !os.IsNotExist(err) {
				t.Fatalf("tracker config created: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != string(stale) {
				t.Fatalf("stale file overwritten: %q %v", got, err)
			}
		})
	}
}

func TestSetupRejectsStaleRetainedHost(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			root := t.TempDir()
			if out, err := runPresetCLI(t, "_setup", "--dir", root, "--host", "claude,dsh", "--tracker", "markdown"); err != nil {
				t.Fatalf("initial dual-host setup: %v %s", err, out)
			}
			config := filepath.Join(root, ".mulix/tracker.json")
			if err := os.Remove(config); err != nil {
				t.Fatal(err)
			}
			const rel = ".dsh/skills/mulix-clarify/SKILL.md"
			const stale = "stale DeepSeek Harness phase skill\n"
			file := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.WriteFile(file, []byte(stale), 0644); err != nil {
				t.Fatal(err)
			}
			args := []string{"_setup", "--dir", root, "--host", "claude", "--tracker", "markdown"}
			if force {
				args = append(args, "--force")
			}
			if _, err := runPresetCLI(t, args...); err == nil || !strings.Contains(err.Error(), rel) {
				t.Errorf("setup should reject retained stale host: %v", err)
			}
			if _, err := os.Stat(config); !os.IsNotExist(err) {
				t.Errorf("setup activated the project layout: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != stale {
				t.Errorf("setup changed unselected host skill: %q %v", got, err)
			}
		})
	}
}

func TestSetupProducesProjectGuidance(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "adr"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "GLOSSARY.md"), []byte("glossary"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "_setup", "--dir", root, "--host", "dsh", "--tracker", "markdown"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs/agents/triage-labels.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"needs-triage", "needs-info", "ready-for-agent", "ready-for-human", "wontfix"} {
		if !strings.Contains(string(data), role) {
			t.Errorf("missing triage role %s", role)
		}
	}
	data, err = os.ReadFile(filepath.Join(root, "docs/agents/domain.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"../GLOSSARY.md", "../adr/"} {
		if !strings.Contains(string(data), link) {
			t.Errorf("missing domain pointer %s", link)
		}
	}
	remote := t.TempDir()
	setupFakeGH(t)
	if _, err := runPresetCLI(t, "_setup", "--dir", remote, "--host", "claude", "--tracker", "github", "--hostname", "github.example.test", "--project", "Team/Repo"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(remote, "docs/agents/triage-labels.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Remote label", "needs-triage", "needs-info", "ready-for-agent", "ready-for-human", "wontfix"} {
		if !strings.Contains(string(data), s) {
			t.Errorf("remote mapping missing %q", s)
		}
	}
	data, err = os.ReadFile(filepath.Join(remote, "docs/agents/domain.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "../../GLOSSARY.md") || !strings.Contains(string(data), "../adr/") {
		t.Errorf("missing default domain links: %s", data)
	}
}

func TestSetupPreservesCRLFManagedGuidance(t *testing.T) {
	root := t.TempDir()
	args := []string{"_setup", "--dir", root, "--host", "claude", "--tracker", "markdown"}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("initial setup: %v %s", err, out)
	}
	for _, rel := range []string{"docs/agents/issue-tracker.md", "docs/agents/triage-labels.md", "docs/agents/domain.md", "CLAUDE.md"} {
		file := filepath.Join(root, rel)
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		converted := strings.ReplaceAll(string(data), "\n", "\r\n") + "User note.\r\n"
		if err := os.WriteFile(file, []byte(converted), 0644); err != nil {
			t.Fatal(err)
		}
	}
	domain := filepath.Join(root, "docs/agents/domain.md")
	before, err := os.ReadFile(domain)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("CRLF rerun: %v %s", err, out)
	}
	after, err := os.ReadFile(domain)
	if err != nil || string(after) != string(before) {
		t.Fatalf("no-op changed bytes: %q %v", after, err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/GLOSSARY.md"), []byte("glossary"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("CRLF update: %v %s", err, out)
	}
	after, err = os.ReadFile(domain)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), "../../GLOSSARY.md", "../GLOSSARY.md", 1)
	if string(after) != want {
		t.Fatalf("replacement changed outside bytes or line endings: %q", after)
	}
	edited := strings.Replace(want, "before using domain terms", "after user changes", 1)
	if err := os.WriteFile(domain, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("edited CRLF guidance accepted")
	}
	after, err = os.ReadFile(domain)
	if err != nil || string(after) != edited {
		t.Fatalf("edited guidance changed: %q %v", after, err)
	}
}

func TestSetupRefreshesDomainGlossaryPointer(t *testing.T) {
	root := t.TempDir()
	args := []string{"_setup", "--dir", root, "--host", "claude", "--tracker", "markdown"}
	if err := os.WriteFile(filepath.Join(root, "GLOSSARY.md"), []byte("root glossary"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	domain := filepath.Join(root, "docs/agents/domain.md")
	initial, err := os.ReadFile(domain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(initial), "../../GLOSSARY.md") {
		t.Fatalf("root pointer missing: %s", initial)
	}
	customized := append(initial, []byte("\nKeep this user note.\n")...)
	if err := os.WriteFile(domain, customized, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/GLOSSARY.md"), []byte("docs glossary"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("repeated setup with new glossary: %v (%s)", err, out)
	}
	refreshed, err := os.ReadFile(domain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(refreshed), "](../GLOSSARY.md)") || !strings.Contains(string(refreshed), "Keep this user note.") {
		t.Fatalf("pointer or user text lost: %s", refreshed)
	}
	if err := os.Remove(filepath.Join(root, "docs/GLOSSARY.md")); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, args...); err != nil {
		t.Fatalf("repeated setup after removing glossary: %v (%s)", err, out)
	}
	refreshed, err = os.ReadFile(domain)
	if err != nil || !strings.Contains(string(refreshed), "](../../GLOSSARY.md)") || !strings.Contains(string(refreshed), "Keep this user note.") {
		t.Fatalf("root pointer or user text lost: %s %v", refreshed, err)
	}
	edited := strings.Replace(string(refreshed), "before using domain terms", "after changing user rules", 1)
	if edited == string(refreshed) {
		t.Fatal("fixture did not edit managed block")
	}
	if err := os.WriteFile(domain, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("edited managed block accepted")
	}
	after, err := os.ReadFile(domain)
	if err != nil || string(after) != edited {
		t.Fatalf("edited guidance changed: %s %v", after, err)
	}
}

func TestSetupUnownedGuidanceBlocksBeforeWrites(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "agents")
	if err := os.MkdirAll(docs, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(docs, "issue-tracker.md")
	before := []byte("Use the existing tracker.\n")
	if err := os.WriteFile(path, before, 0644); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(user, []byte("keep me\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "_setup", "--dir", root, "--host", "claude,dsh", "--tracker", "markdown"); err == nil {
		t.Fatal("unowned guidance accepted")
	}
	for _, rel := range []string{".mulix", ".claude", ".dsh", "AGENTS.md", "docs/agents/domain.md", "docs/agents/triage-labels.md"} {
		if _, err := os.Lstat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("preflight changed %s: %v", rel, err)
		}
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(before) {
		t.Fatalf("tracker guidance changed: %s %v", got, err)
	}
	if got, err := os.ReadFile(user); err != nil || string(got) != "keep me\n" {
		t.Fatalf("user guidance changed: %s %v", got, err)
	}
}

func TestSetupStandalone(t *testing.T) {
	root := t.TempDir()
	// The setup skill is distributed separately and can be copied before any installation.
	skill, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", "setup-mulix", "SKILL.md"))
	if err != nil {
		t.Fatalf("standalone setup skill unavailable: %v", err)
	}
	standalone := filepath.Join(root, ".claude", "skills", "setup-mulix", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(standalone), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(standalone, skill, 0644); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(standalone)
	if err != nil {
		t.Fatal(err)
	}
	front := strings.SplitN(string(installed), "---", 3)
	if len(front) != 3 {
		t.Fatal("skill frontmatter missing")
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(front[1]), &meta); err != nil || meta.Name != filepath.Base(filepath.Dir(standalone)) || strings.TrimSpace(meta.Description) == "" {
		t.Fatalf("skill is not discoverable: %+v, %v", meta, err)
	}
	var args []string
	for _, block := range strings.Split(front[2], "```") {
		if strings.HasPrefix(block, "sh\n") {
			args = strings.Fields(strings.TrimPrefix(block, "sh\n"))
			break
		}
	}
	if len(args) < 2 || args[0] != "mulix" {
		t.Fatalf("skill lacks an executable setup example: %v", args)
	}
	args = args[1:]
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--dir":
			args[i+1] = root
		case "--hostname":
			args[i+1] = "git.example.test"
		case "--project":
			args[i+1] = "group/project"
		}
	}
	t.Setenv("PATH", t.TempDir())
	args = append(args, "--allow-pending")
	out, err := runPresetCLI(t, args...)
	if err != nil {
		t.Fatalf("standalone setup bridge failed: %v; output: %s", err, out)
	}
	for _, rel := range []string{".claude/skills/mulix-using/SKILL.md", ".dsh/skills/mulix-using/SKILL.md", ".mulix/tracker.json"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("setup did not install %s: %v", rel, err)
		}
	}
	for _, rel := range []string{".dsh/skills/setup-mulix", ".mulix/.installed/.claude/skills/setup-mulix"} {
		if _, err := os.Lstat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("setup skill must not be installed or registered by the bridge: %s: %v", rel, err)
		}
	}
	if got, err := os.ReadFile(standalone); err != nil || string(got) != string(skill) {
		t.Fatalf("setup changed the manually installed skill: %v", err)
	}
	if _, err := runPresetCLI(t, "init", "--dir", t.TempDir()); err == nil {
		t.Error("legacy init must not be registered")
	}
	version, err := runPresetCLI(t, "version")
	if err != nil || !strings.HasPrefix(version, "mulix ") {
		t.Fatalf("skill cannot check binary version: %q %v", version, err)
	}
	help, err := runPresetCLI(t, "--help")
	if err != nil || strings.Contains(help, "_setup") {
		t.Errorf("setup bridge must be hidden from help: %s (%v)", help, err)
	}
}

func TestSetupPreservesUserDocs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("# User instructions\nKeep my rules.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "_setup", "--dir", root, "--host", "claude", "--tracker", "markdown"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(got), "Keep my rules.") {
		t.Fatalf("user instructions lost: %s (%v)", got, err)
	}
}

func TestSetupPendingRequiresExplicitChoice(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	userFile := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(userFile, []byte("keep user instructions\n"), 0644); err != nil {
		t.Fatal(err)
	}
	args := []string{"_setup", "--dir", root, "--host", "dsh", "--tracker", "gitlab", "--hostname", "git.example.test", "--project", "team/project"}
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("unreachable tracker accepted without explicit permission")
	}
	if _, err := os.Stat(filepath.Join(root, ".mulix/tracker.json")); !os.IsNotExist(err) {
		t.Fatalf("failed probe saved configuration: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "CLAUDE.md" {
		t.Fatalf("refused probe changed project: %v", entries)
	}
	if b, err := os.ReadFile(userFile); err != nil || string(b) != "keep user instructions\n" {
		t.Fatalf("refused probe changed user file: %q %v", b, err)
	}
	if _, err := runPresetCLI(t, append(args, "--allow-pending")...); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mulix/tracker.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "git.example.test") || strings.Contains(string(data), "token") {
		t.Fatalf("unexpected pending config %s", data)
	}
	config, found, err := tracker.LoadConfig(root)
	if err != nil || !found || config.ProjectID != "" || config.Project != "team/project" {
		t.Fatalf("pending target is incorrect: %+v, %v, %v", config, found, err)
	}
}

func TestSetupRejectsLinkedInstructionDirectory(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "docs")); err != nil {
		t.Skip(err)
	}
	if _, err := runPresetCLI(t, "_setup", "--dir", root, "--host", "claude", "--tracker", "markdown"); err == nil {
		t.Fatal("setup followed a symlink outside the project")
	}
	if _, err := os.Stat(filepath.Join(outside, "agents")); !os.IsNotExist(err) {
		t.Fatalf("setup wrote outside the project: %v", err)
	}
}

func TestSetupRepeatAndConflictProtection(t *testing.T) {
	root := t.TempDir()
	args := []string{"_setup", "--dir", root, "--host", "dsh", "--tracker", "markdown"}
	if _, err := runPresetCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".dsh/skills/brainstorming/SKILL.md")
	if err := os.WriteFile(path, []byte("custom user skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, args...); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "custom user skill\n" {
		t.Fatalf("custom content lost: %s, %v", data, err)
	}
	docs := filepath.Join(root, "docs/agents/issue-tracker.md")
	if err := os.WriteFile(docs, []byte("manually modified guidance\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, args...); err == nil {
		t.Fatal("changed guidance accepted without reconciliation")
	}
}

func TestInitCmd_HostDSHInstallsDSHTree(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "_setup", "--tracker", "markdown", "--dir", ".", "--host", "dsh")
	if err != nil {
		t.Fatalf("init --host dsh: %v (output: %s)", err, out)
	}

	for _, rel := range []string{
		filepath.Join(".dsh", "skills", "mulix-using", "SKILL.md"),
		filepath.Join(".dsh", "skills", "brainstorming", "SKILL.md"),
		filepath.Join("AGENTS.md"),
	} {
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error("expected --host dsh to not write .claude/settings.json")
	}
	data, err := os.ReadFile(filepath.Join("AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "`.dsh/skills/`") {
		t.Errorf("expected AGENTS.md to name the dsh skills dir, got:\n%s", data)
	}
	if !strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("expected the DSH note in the output, got: %s", out)
	}
}

func TestInitCmd_HostClaudeKeepsClaudeNote(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "_setup", "--tracker", "markdown", "--dir", ".", "--host", "claude")
	if err != nil {
		t.Fatalf("init --host claude: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "Claude Code") {
		t.Errorf("expected the Claude Code note in the output, got: %s", out)
	}
	if strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("did not expect the DSH note for a claude-only init, got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.json")); err != nil {
		t.Errorf("expected the Claude hook settings to exist: %v", err)
	}
}

func TestInitCmd_BothHosts(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "_setup", "--tracker", "markdown", "--dir", ".", "--host", "claude,dsh")
	if err != nil {
		t.Fatalf("init --host claude,dsh: %v (output: %s)", err, out)
	}
	for _, rel := range []string{
		filepath.Join(".claude", "skills", "mulix-using", "SKILL.md"),
		filepath.Join(".dsh", "skills", "mulix-using", "SKILL.md"),
		filepath.Join(".claude", "settings.json"),
		filepath.Join("AGENTS.md"),
	} {
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if !strings.Contains(out, "Claude Code") || !strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("expected both host notes in the output, got: %s", out)
	}
}

func TestInitCmd_UnknownHostFails(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "_setup", "--tracker", "markdown", "--dir", ".", "--host", "codex")
	if err == nil {
		t.Fatalf("expected an error for an unknown host, output: %s", out)
	}
	if !strings.Contains(err.Error()+out, "unknown host") {
		t.Errorf("expected the error to name the unknown host, got err=%v out=%s", err, out)
	}
}
