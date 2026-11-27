package quality

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKarateReportsNoUnaccountedErrorsAndEveryFeatureExecutes(t *testing.T) {
	const root = ".mulix/.runtime/smoke/tests"
	const first = "features/task-1/a.feature"
	const final = "features/final/integration.feature"
	type data = map[string]any
	parts := func(features []string, skippedFinal, unexpectedError bool) ([]byte, []string) {
		var lines []string
		emit := func(kind string, obj data) {
			b, err := json.Marshal(data{"type": kind, "data": obj})
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, string(b))
		}
		emit("SUITE_ENTER", data{})
		files := make([]string, 0, len(features))
		passed, failed, skipped := 0, 0, 0
		for i, file := range features {
			files = append(files, root+"/"+file)
			item := data{"feature": file, "name": "owned", "refId": "[1:3]", "executionIndex": i + 1, "callDepth": 0}
			emit("SCENARIO_ENTER", item)
			if unexpectedError {
				emit("ERROR", data{"feature": file, "scenario": "owned", "type": "AssertionError"})
				failed++
			} else {
				passed++
			}
			item["passed"] = !unexpectedError
			item["skipped"] = skippedFinal && file == final
			if item["skipped"] == true {
				skipped++
			}
			emit("SCENARIO_EXIT", item)
		}
		if unexpectedError {
			emit("ERROR", data{"type": "IOException", "message": "report output became inaccessible"})
		}
		emit("SUITE_EXIT", data{"summary": data{"scenarioCount": len(features), "scenariosPassed": passed, "scenariosFailed": failed, "scenariosSkipped": skipped}})
		return []byte(strings.Join(lines, "\n") + "\n"), files
	}
	for _, tc := range []struct {
		name           string
		features       []string
		skipped, infra bool
	}{
		{"late infrastructure error invalidates RED", []string{first}, false, true},
		{"skip-only final entry is not executed", []string{first, final}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream, files := parts(tc.features, tc.skipped, tc.infra)
			if _, err := parseKarateEvents(stream, root, files); err == nil {
				t.Fatal("incomplete scope or hidden infrastructure error accepted")
			}
		})
	}
}

func TestKarateStagedSourcesCanBeCheckedAgainstPreRunSnapshot(t *testing.T) {
	p := karateFixture(t)
	_, suite := pinnedKarateFixtureSuite(t, p)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	before, err := ExecutionFingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	original, err := prepareKarate(p, c, suite, 1, t.TempDir(), "original")
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, karateHelper, "temporarily changed helper\n")
	changed, err := prepareKarate(p, c, suite, 1, t.TempDir(), "changed")
	if err != nil {
		t.Fatal(err)
	}
	if got := executeSuite(t.Context(), p, c, suite, 1, t.TempDir(), "stale", before); got.ExitCode != -1 || !strings.Contains(got.Error, "pre-run snapshot") {
		t.Fatalf("staged source drift did not stop the JVM launch: %+v", got)
	}
	put(t, p.Root, karateHelper, "fixture\n")
	after, err := ExecutionFingerprint(p, c, 1)
	if err != nil || before != after {
		t.Fatalf("fixture did not restore the same inputs: %v", err)
	}
	if want := before.KarateSources; want == "" || original.KarateSources != want || changed.KarateSources == want {
		t.Fatal("receipt could bind original source without detecting different staged bytes")
	}
}
