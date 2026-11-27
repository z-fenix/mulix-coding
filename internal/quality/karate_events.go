package quality

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

type karateCaseEvent struct {
	Feature string `json:"feature"`
	Name    string `json:"name"`
	RefID   string `json:"refId"`
	Index   int    `json:"executionIndex"`
	Depth   *int   `json:"callDepth"`
	Passed  *bool  `json:"passed"`
	Skipped *bool  `json:"skipped"`
}

type karateResultSummary struct {
	Count   *int `json:"scenarioCount"`
	Passed  *int `json:"scenariosPassed"`
	Failed  *int `json:"scenariosFailed"`
	Skipped *int `json:"scenariosSkipped"`
}

// Read only the event file created by this execution, never text/HTML/XML
// summaries. RC1 may count skipped cases as passed even with exit zero.
func readKarateEvents(p Project, inv karateInvocation) ([]TestResult, string, error) {
	file, err := karateEventPath(inv)
	if err != nil {
		return nil, "", err
	}
	f, _, err := openArtifact(file, outputLimit)
	if err != nil {
		return nil, "", fmt.Errorf("open fresh Karate event stream: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, outputLimit+1))
	if err != nil || len(data) > outputLimit || len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, "", fmt.Errorf("missing, truncated or oversized Karate event stream")
	}
	results, err := parseKarateEvents(data, runtimeTestsRoot(p), inv.Features)
	return results, Digest(data), err
}

func parseKarateEvents(data []byte, root string, features []string) ([]TestResult, error) {
	files := make(map[string]bool, len(features))
	for _, full := range features {
		files[strings.TrimPrefix(full, root+"/")] = false
	}
	pending := map[string]karateCaseEvent{}
	helpers := map[string]int{}
	results := map[string]TestResult{}
	asserted := map[string]bool{}
	var started, ended bool
	var summary karateResultSummary
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 4096), 4<<20)
	for scan.Scan() {
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scan.Bytes(), &event); err != nil || event.Type == "" {
			return nil, fmt.Errorf("invalid Karate event")
		}
		if ended || (!started && event.Type != "SUITE_ENTER") {
			return nil, fmt.Errorf("event outside the Karate suite")
		}
		switch event.Type {
		case "SUITE_ENTER":
			if started {
				return nil, fmt.Errorf("duplicate Karate suite start")
			}
			started = true
		case "SUITE_EXIT":
			var terminal struct {
				Summary karateResultSummary `json:"summary"`
			}
			if err := json.Unmarshal(event.Data, &terminal); err != nil {
				return nil, fmt.Errorf("invalid Karate suite summary: %w", err)
			}
			summary, ended = terminal.Summary, true
		case "SCENARIO_ENTER", "SCENARIO_EXIT":
			var sc karateCaseEvent
			if err := json.Unmarshal(event.Data, &sc); err != nil || sc.Depth == nil || *sc.Depth < 0 {
				return nil, fmt.Errorf("invalid Karate scenario data")
			}
			sc.Feature = strings.ReplaceAll(sc.Feature, `\`, "/")
			id := root + "/" + sc.Feature + "#" + sc.Name
			if *sc.Depth != 0 {
				// Track helper boundaries only to attribute errors; their
				// executions never count as enrolled acceptance cases.
				if event.Type == "SCENARIO_ENTER" {
					helpers[id]++
				} else if helpers[id] == 1 {
					delete(helpers, id)
				} else if helpers[id] > 1 {
					helpers[id]--
				} else {
					return nil, fmt.Errorf("unpaired Karate helper event")
				}
				continue
			}
			if _, ok := files[sc.Feature]; !ok || strings.TrimSpace(sc.Name) == "" || sc.RefID == "" || sc.Index <= 0 {
				return nil, fmt.Errorf("Karate reported an unselected or unidentifiable scenario")
			}
			old, open := pending[id]
			if results[id].Name != "" || (event.Type == "SCENARIO_ENTER" && open) {
				return nil, fmt.Errorf("duplicate Karate scenario %s", id)
			}
			if event.Type == "SCENARIO_ENTER" {
				if len(pending)+len(results) >= 20000 {
					return nil, fmt.Errorf("too many Karate scenarios")
				}
				pending[id] = sc
				continue
			}
			if !open || old.RefID != sc.RefID || old.Index != sc.Index || sc.Passed == nil || sc.Skipped == nil {
				return nil, fmt.Errorf("unpaired or incomplete Karate scenario: %s", id)
			}
			status := "fail"
			if *sc.Skipped {
				status = "skip"
			} else if *sc.Passed {
				status = "pass"
			}
			results[id] = TestResult{Name: id, Status: status, Assertion: status == "fail" && asserted[id]}
			if status != "skip" {
				files[sc.Feature] = true
			}
			delete(pending, id)
		case "ERROR":
			var failure struct{ Feature, Scenario, Type string }
			if err := json.Unmarshal(event.Data, &failure); err != nil {
				return nil, fmt.Errorf("invalid Karate error event: %w", err)
			}
			id := root + "/" + strings.ReplaceAll(failure.Feature, `\`, "/") + "#" + failure.Scenario
			_, open := pending[id]
			if !open && helpers[id] > 0 {
				continue // The enclosing scenario owns the observable outcome.
			}
			if !open || helpers[id] != 0 {
				return nil, fmt.Errorf("unaccounted Karate error outside a unique executing scenario")
			}
			prior, seen := asserted[id]
			asserted[id] = failure.Type == "AssertionError" && (!seen || prior)
		}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if !started || !ended || len(pending) > 0 || len(helpers) > 0 || len(results) == 0 {
		return nil, fmt.Errorf("Karate did not complete a nonempty suite")
	}
	for file, seen := range files {
		if !seen {
			return nil, fmt.Errorf("Karate feature executed no scenarios: %s", file)
		}
	}
	var out []TestResult
	counts := map[string]int{}
	for _, r := range results {
		out = append(out, r)
		counts[r.Status]++
	}
	if summary.Count == nil || summary.Passed == nil || summary.Failed == nil || summary.Skipped == nil ||
		*summary.Count != len(out) || *summary.Failed != counts["fail"] || *summary.Skipped != counts["skip"] ||
		(*summary.Passed != counts["pass"] && *summary.Passed != counts["pass"]+counts["skip"]) ||
		counts["pass"]+counts["fail"] == 0 {
		return nil, fmt.Errorf("Karate suite counts do not establish actual scenario execution")
	}
	slices.SortFunc(out, func(a, b TestResult) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}
