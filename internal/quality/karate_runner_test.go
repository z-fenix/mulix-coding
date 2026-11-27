package quality

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Use this test binary as a portable process fixture. The real-JAR smoke
// test separately checks the contract against the specified distribution.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "-Duser.home=") {
		os.Exit(karateProcessFixture(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func karateProcessFixture(args []string) int {
	var out, work string
	var entries []string
	for i, a := range args {
		switch a {
		case "-o":
			out = args[i+1]
		case "-w":
			work = args[i+1]
		default:
			if strings.HasSuffix(a, ".feature") {
				entries = append(entries, a)
			}
		}
	}
	cwd, _ := os.Getwd()
	if out == "" || work == "" || filepath.Clean(cwd) != filepath.Clean(work) || len(entries) != 1 || !filepath.IsAbs(entries[0]) || os.Getenv("JAVA_TOOL_OPTIONS") != "" || os.Getenv("KARATE_OPTIONS") != "" {
		return 2
	}
	if log := os.Getenv("KARATE_LOG_FILE"); !filepath.IsAbs(log) || os.WriteFile(log, []byte("rolling fixture log\n"), 0600) != nil {
		return 2
	}
	if err := os.MkdirAll(filepath.Join(out, "karate-json"), 0700); err != nil {
		return 2
	}
	f, err := os.Create(filepath.Join(out, "karate-json", "karate-events.jsonl"))
	if err != nil {
		return 2
	}
	defer f.Close()
	emit := func(kind string, d any) {
		_ = json.NewEncoder(f).Encode(map[string]any{"type": kind, "timeStamp": 1, "threadId": 1, "data": d})
	}
	b, _ := os.ReadFile(entries[0])
	mode := strings.TrimSpace(string(b))
	if mode == "overflow" {
		for range outputLimit/(1<<20) + 1 {
			_, _ = os.Stdout.Write(make([]byte, 1<<20))
		}
		return 1
	}
	rel, _ := filepath.Rel(work, entries[0])
	path := filepath.ToSlash(rel)
	name := "保存成功"
	caseData := map[string]any{"feature": path, "name": name, "executionIndex": 1, "refId": "[1:3]", "callDepth": 0}
	emit("SUITE_ENTER", map[string]any{})
	passed, failed, skip, count := 1, 0, 0, 1
	if mode == "empty" {
		passed, count = 0, 0
	} else if mode == "configuration" {
		passed, failed = 0, 1 // RC1 can report a failure without a case enter/exit.
	} else {
		emit("SCENARIO_ENTER", caseData)
		helper := map[string]any{"feature": "helpers/setup.feature", "name": "helper", "executionIndex": 2, "refId": "[1:3:1]", "callDepth": 1, "passed": true, "skipped": false}
		emit("SCENARIO_ENTER", helper)
		emit("SCENARIO_EXIT", helper)
		if mode == "assertion" || mode == "unknown" {
			passed, failed = 0, 1
			kind := "AssertionError"
			if mode == "unknown" {
				kind = "IOException"
			}
			emit("ERROR", map[string]any{"feature": path, "scenario": name, "type": kind})
		}
		if mode == "dry" {
			skip = 1 // RC1 may also count the skipped case as passed.
		}
		caseData["passed"], caseData["skipped"] = failed == 0, skip != 0
		emit("SCENARIO_EXIT", caseData)
		if mode == "duplicate" {
			emit("SCENARIO_EXIT", caseData)
		}
	}
	if mode != "truncated" {
		emit("SUITE_EXIT", map[string]any{"summary": map[string]any{"scenarioCount": count, "scenariosPassed": passed, "scenariosFailed": failed, "scenariosSkipped": skip}})
	}
	if failed != 0 {
		return 1
	}
	return 0
}

func setKarateProcessFixture(t *testing.T, p Project) string {
	t.Helper()
	root := t.TempDir()
	java := filepath.Join(root, "bin", "java")
	if runtime.GOOS == "windows" {
		java += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(java), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(java, data, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, root, "release", "JAVA_VERSION=\"21.0.12.1\"\n")
	jar := filepath.Join(root, "fixture.jar")
	f, err := os.Create(jar)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	e, err := w.Create("META-INF/maven/io.karatelabs.ext/karate-agent/pom.properties")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(e, "artifactId=karate-agent\ngroupId=io.karatelabs.ext\nversion=2.1.5.RC1\n")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(jar)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ReadProjectFile(p.Root, PolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, PolicyPath, strings.Replace(string(policy), strings.Repeat("a", 64), Digest(data), 1))
	t.Setenv("MULIX_KARATE_JAVA", java)
	t.Setenv("MULIX_KARATE_JAR", jar)
	t.Setenv("JAVA_TOOL_OPTIONS", "forbidden")
	t.Setenv("KARATE_OPTIONS", "--dryrun")
	return jar
}

func TestRunKarateValidatesStructuredScenarioEvidence(t *testing.T) {
	p := karateFixture(t)
	jar := setKarateProcessFixture(t, p)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode, stage string
		ok          bool
	}{{"pass", "green", true}, {"assertion", "red", true}, {"empty", "green", false}, {"dry", "green", false}, {"duplicate", "green", false}, {"configuration", "red", false}, {"unknown", "red", false}, {"truncated", "green", false}, {"overflow", "red", false}} {
		t.Run(tc.mode, func(t *testing.T) {
			put(t, p.Root, karateA, tc.mode)
			receipt, err := Run(context.Background(), p, c, policy, 1, tc.stage, t.TempDir())
			if (err == nil) != tc.ok || receipt.Success != tc.ok {
				t.Fatalf("%s %s: success=%t error=%v", tc.mode, tc.stage, receipt.Success, err)
			}
			if tc.ok && (len(receipt.Suites) != 1 || len(receipt.Suites[0].Tests) != 1 || receipt.Suites[0].Tests[0].Name != karateA+"#保存成功") {
				t.Fatalf("a non-owned or helper scenario entered the task receipt: %+v", receipt.Suites)
			}
		})
	}
	if err := os.WriteFile(jar, []byte("substituted"), 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := Run(context.Background(), p, c, policy, 1, "green", t.TempDir()); err == nil || r.Success {
		t.Fatal("different artifact produced a passing receipt")
	}
}
