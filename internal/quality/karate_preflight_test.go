package quality

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pinnedKarateFixtureSuite(t *testing.T, p Project) (string, Suite) {
	t.Helper()
	jar := setKarateProcessFixture(t, p)
	policy, err := LoadPolicy(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	return jar, policy.Suites[0]
}

func TestKaratePreflightRejectsWrongRuntime(t *testing.T) {
	p := karateFixture(t)
	jar, suite := pinnedKarateFixtureSuite(t, p)
	java := os.Getenv("MULIX_KARATE_JAVA")
	for _, tc := range []struct {
		name, key, value, want string
	}{
		{"jar must be absolute", "MULIX_KARATE_JAR", "relative.jar", "absolute path"},
		{"missing jar", "MULIX_KARATE_JAR", jar + ".missing", "open Karate JAR"},
		{"nonregular jar", "MULIX_KARATE_JAR", filepath.Dir(jar), "regular input"},
		{"java must be absolute", "MULIX_KARATE_JAVA", "other/java", "absolute"},
		{"missing explicit java", "MULIX_KARATE_JAVA", java + ".missing", "resolve Java executable"},
		{"missing JDK metadata", "MULIX_KARATE_JAVA", jar, "release metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := inspectKarate(suite); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	t.Run("no default java", func(t *testing.T) {
		t.Setenv("MULIX_KARATE_JAVA", "")
		t.Setenv("PATH", t.TempDir())
		if _, err := inspectKarate(suite); err == nil || !strings.Contains(err.Error(), "Java 21+") {
			t.Fatalf("unavailable Java: %v", err)
		}
	})
	t.Run("old JVM", func(t *testing.T) {
		put(t, filepath.Dir(filepath.Dir(java)), "release", "JAVA_VERSION=\"17.0.4\"\n")
		if _, err := inspectKarate(suite); err == nil || !strings.Contains(err.Error(), "21 or newer") {
			t.Fatalf("old JVM accepted: %v", err)
		}
		put(t, filepath.Dir(filepath.Dir(java)), "release", "JAVA_VERSION=\"21.0.12.1\"\n")
	})
	for _, tc := range []struct{ name, metadata, text string }{
		{"invalid zip", "", "not an archive"},
		{"wrong coordinate", "other/pom.properties", "version=2.1.5.RC1"},
		{"wrong version", mavenKarateMetadata, "version=2.1.4"},
		{"missing version", mavenKarateMetadata, "artifactId=karate-agent"},
		{"oversized metadata", mavenKarateMetadata, strings.Repeat("X", 4097)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "runtime.jar")
			f, err := os.Create(file)
			if err != nil {
				t.Fatal(err)
			}
			if tc.metadata == "" {
				if _, err := io.WriteString(f, tc.text); err != nil {
					t.Fatal(err)
				}
			} else {
				z := zip.NewWriter(f)
				entry, err := z.Create(tc.metadata)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(entry, tc.text); err != nil {
					t.Fatal(err)
				}
				if err := z.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			candidate := suite
			candidate.Karate = &KaratePolicy{Version: KarateVersion, SHA256: Digest(b)}
			t.Setenv("MULIX_KARATE_JAR", file)
			if _, err := inspectKarate(candidate); err == nil || !strings.Contains(err.Error(), "approved version") {
				t.Fatalf("unqualified artifact identified as RC1: %v", err)
			}
		})
	}
}

func TestKaratePolicyBindsOnlyExplicitEnvironment(t *testing.T) {
	p := karateFixture(t)
	_, suite := pinnedKarateFixtureSuite(t, p)
	for _, values := range [][]string{{"KARATE_LOG_FILE"}, {"JAVA_TOOL_OPTIONS"}, {"JDK_JAVA_OPTIONS"}, {"A-B"}, {"APP_URL", "app_URL"}} {
		suite.Karate.Env = values
		data, err := json.Marshal(Policy{Schema: "mulix.quality.v1", Suites: []Suite{suite}})
		if err != nil {
			t.Fatal(err)
		}
		put(t, p.Root, PolicyPath, string(data))
		if _, err := LoadPolicy(p.Root); err == nil {
			t.Fatalf("unsafe or ambiguous environment declaration accepted: %v", values)
		}
	}
	suite.Karate.Env = []string{"APP_URL"}
	stamp := func(url string) karateTool {
		t.Helper()
		t.Setenv("APP_URL", url)
		stamp, err := inspectKarate(suite)
		if err != nil {
			t.Fatal(err)
		}
		return stamp
	}
	a, b := stamp("http://127.0.0.1:9001"), stamp("http://127.0.0.1:9002")
	if a.Digest == b.Digest || len(b.ProjectEnv) != 1 || b.ProjectEnv[0] != "APP_URL=http://127.0.0.1:9002" {
		t.Fatal("explicit environment is not bound independently of the inherited JVM variables")
	}
}

func TestKarateVerifyInputsRejectMissingCaseOrGeneratedOutput(t *testing.T) {
	for _, mode := range []string{"missing", "generated"} {
		t.Run(mode, func(t *testing.T) {
			p := karateFixture(t)
			c, err := LoadContract(p)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing" {
				if err := os.Remove(filepath.Join(p.Root, filepath.FromSlash(karateFinal))); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, p.Root, runtimeTestsRoot(p)+"/target/karate-agent.log", "run output\n")
			}
			if _, err := Fingerprint(p, c, 0); err == nil {
				t.Fatal("invalid retained final input was allowed")
			}
			if _, err := Fingerprint(p, c, 1); err != nil {
				t.Fatalf("unselected final scope broke a task run: %v", err)
			}
		})
	}
}

func TestKarateReadEventFileRequiresFreshCompleteOutput(t *testing.T) {
	p := karateFixture(t)
	inv := karateInvocation{RunDir: t.TempDir(), Features: []string{karateA}}
	if _, _, err := readKarateEvents(p, inv); err == nil {
		t.Fatal("accepted missing evidence")
	}
	put(t, inv.RunDir, "output/karate-json/karate-events.jsonl", `{"type":"SUITE_EXIT"}`)
	if _, _, err := readKarateEvents(p, inv); err == nil {
		t.Fatal("accepted partial evidence without final newline")
	}
	badRoot := t.TempDir()
	put(t, badRoot, "output", "not a directory")
	inv.RunDir = badRoot
	if _, _, err := readKarateEvents(p, inv); err == nil {
		t.Fatal("accepted evidence outside its output tree")
	}
}

func TestKarateMappingCannotUseSharedSuiteOrOrdinaryFile(t *testing.T) {
	p := karateFixture(t)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	pol, err := LoadPolicy(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	other := pol.Suites[0]
	other.ID = "second-karate"
	duplicate := Policy{Schema: pol.Schema, Suites: []Suite{pol.Suites[0], other}}
	data, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, PolicyPath, string(data))
	if _, err := LoadPolicy(p.Root); err == nil {
		t.Fatal("two suites could both run the final Karate scope")
	}
	if err := ValidateMapping(c, duplicate); err == nil {
		t.Fatal("duplicate Karate suite ownership accepted")
	}
	c.Cases[0].File = "ordinary.feature"
	if err := ValidateMapping(c, pol); err == nil {
		t.Fatal("Karate case escaped the change-owned runtime test directory")
	}
}

func TestKarateRejectsUnframedOrUnselectedScenarioEvents(t *testing.T) {
	const start = `{"type":"SUITE_ENTER","data":{}}` + "\n"
	const enter = `{"type":"SCENARIO_ENTER","data":{"feature":"features/task-1/a.feature","name":"one","refId":"[1:3]","executionIndex":1,"callDepth":0}}` + "\n"
	const end = `{"type":"SCENARIO_EXIT","data":{"feature":"features/task-1/a.feature","name":"one","refId":"[1:3]","executionIndex":1,"callDepth":0,"passed":true,"skipped":false}}` + "\n"
	const complete = `{"type":"SUITE_EXIT","data":{"summary":{"scenarioCount":1,"scenariosPassed":1,"scenariosFailed":0,"scenariosSkipped":0}}}` + "\n"
	const root = ".mulix/.runtime/smoke/tests"
	entries := []string{root + "/features/task-1/a.feature"}
	if got, err := parseKarateEvents([]byte(start+enter+end+complete), root, entries); err != nil || len(got) != 1 || got[0].Status != "pass" {
		t.Fatalf("expected a paired real result: %+v %v", got, err)
	}
	for name, data := range map[string]string{
		"malformed event":      start + "{\n" + complete,
		"duplicate start":      start + start + enter + end + complete,
		"no suite start":       enter + end + complete,
		"late event":           start + enter + end + complete + start,
		"bad terminal":         start + enter + end + `{"type":"SUITE_EXIT","data":42}` + "\n",
		"foreign feature":      start + strings.Replace(enter, "task-1", "task-2", 1) + end + complete,
		"missing call depth":   start + strings.Replace(enter, `,"callDepth":0`, "", 1) + end + complete,
		"mismatched reference": start + enter + strings.Replace(end, "[1:3]", "[1:9]", 1) + complete,
		"bad error event":      start + enter + `{"type":"ERROR","data":12}` + "\n" + end + complete,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseKarateEvents([]byte(data), root, entries); err == nil {
				t.Fatal("incomplete scenario envelope accepted")
			}
		})
	}
	if _, err := parseKarateEvents([]byte(start+enter+end+complete), root, append(entries, root+"/features/task-1/not-run.feature")); err == nil {
		t.Fatal("one entry pass concealed another unexecuted entry")
	}
}
