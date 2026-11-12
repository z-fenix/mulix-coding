package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQualityProcess(t *testing.T) {
	if os.Getenv("MULIX_QUALITY_PROCESS") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	event := func(status string) { fmt.Printf("{\"name\":\"fixture#TestValue\",\"status\":%q}\n", status) }
	switch mode {
	case "overflow-red-stdout", "overflow-red-stderr":
		failure := []byte("{\"name\":\"fixture#TestValue\",\"status\":\"fail\"}\n")
		_, _ = os.Stdout.Write(failure)
		stream, paddingSize := os.Stdout, outputLimit-len(failure)
		if mode == "overflow-red-stderr" {
			stream, paddingSize = os.Stderr, outputLimit
		}
		padding := bytes.Repeat([]byte{'\n'}, paddingSize)
		for len(padding) > 0 {
			n := min(len(padding), 8192)
			_, _ = stream.Write(padding[:n])
			padding = padding[n:]
		}
		_, _ = stream.Write([]byte("invalid output beyond capture limit\n"))
		os.Exit(1)
	case "value":
		data, _ := os.ReadFile("value.go")
		if strings.Contains(string(data), "implemented") {
			event("pass")
		} else {
			event("fail")
			os.Exit(1)
		}
	case "pass":
		event("pass")
	case "fail":
		event("fail")
		os.Exit(1)
	case "false-pass":
		event("pass")
		fmt.Fprintln(os.Stderr, "PASS")
		os.Exit(1)
	case "empty":
	case "skip":
		event("skip")
	case "missing":
		fmt.Println(`{"name":"fixture#TestOther","status":"pass"}`)
	case "malformed":
		fmt.Println("PASS")
	case "duplicate":
		event("pass")
		event("pass")
	case "mutate":
		_ = os.WriteFile("value.go", []byte("changed by command"), 0600)
		event("pass")
	case "sleep":
		time.Sleep(10 * time.Second)
	case "go-build-and-fail":
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"fail","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"fail","Package":"fixture"}`)
		fmt.Println(`{"Action":"fail","Package":"broken-dependency"}`)
		os.Exit(1)
	case "go-cached":
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"pass","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"output","Package":"fixture","Output":"ok  \tfixture\t(cached)\n"}`)
		fmt.Println(`{"Action":"pass","Package":"fixture"}`)
	case "go-restarted-incomplete":
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"pass","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
	case "go-pass":
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"pass","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"pass","Package":"fixture"}`)
	case "go-fail":
		fmt.Println(`{"Action":"run","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"fail","Package":"fixture","Test":"TestValue"}`)
		fmt.Println(`{"Action":"fail","Package":"fixture"}`)
		os.Exit(1)
	}
	os.Exit(0)
}
func processPolicy(t *testing.T, p Project, mode string) Policy {
	t.Helper()
	t.Setenv("MULIX_QUALITY_PROCESS", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	format := "json"
	if strings.HasPrefix(mode, "go-") {
		format = "go-json"
	}
	policy := Policy{Schema: "mulix.quality.v1", Suites: []Suite{{ID: "unit", Command: []string{exe, "-test.run=^TestQualityProcess$", "--", mode}, CWD: ".", Format: format, TimeoutSeconds: 5}}}
	data, err := yaml.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, PolicyPath, string(data))
	return policy
}
func TestRunRejectsFalseSuccess(t *testing.T) {
	for _, mode := range []string{"false-pass", "empty", "skip", "missing", "malformed", "duplicate", "mutate", "go-cached", "go-restarted-incomplete"} {
		t.Run(mode, func(t *testing.T) {
			p := fixture(t)
			c, _ := LoadContract(p)
			r, err := Run(context.Background(), p, c, processPolicy(t, p, mode), 1, "green", t.TempDir())
			if err == nil || r.Success {
				t.Fatalf("%s accepted: %+v, %v", mode, r, err)
			}
		})
	}
}
func TestRunRejectsOutputOverflowDespiteFailingMappedTest(t *testing.T) {
	for _, mode := range []string{"overflow-red-stdout", "overflow-red-stderr"} {
		t.Run(mode, func(t *testing.T) {
			p := fixture(t)
			c, _ := LoadContract(p)
			r, err := Run(context.Background(), p, c, processPolicy(t, p, mode), 1, "red", t.TempDir())
			if err == nil || r.Success {
				t.Fatal("capture overflow accepted as behavioral RED")
			}
			if !strings.Contains(r.Error, "exceeds") {
				t.Fatalf("capture failure not preserved: %v", err)
			}
		})
	}
}

func TestRunRejectsREDWithUnrelatedPackageBuildFailure(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	r, err := Run(context.Background(), p, c, processPolicy(t, p, "go-build-and-fail"), 1, "red", t.TempDir())
	if err == nil || r.Success {
		t.Fatal("a mapped test failure hid another package's build failure")
	}
}

func TestRunActualProcessRedGreenAndVerification(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	red, err := Run(context.Background(), p, c, processPolicy(t, p, "value"), 1, "red", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, "value.go", "package fixture\n// implemented\n")
	green, err := Run(context.Background(), p, c, processPolicy(t, p, "value"), 1, "green", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !red.Success || red.Suites[0].ExitCode == 0 || !green.Success || green.Suites[0].Passed != 1 {
		t.Fatalf("not actual RED/GREEN: %+v %+v", red, green)
	}
	if err := ValidateTaskEvidence(p, c, []RunRecord{red, green}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTaskEvidence(p, c, []RunRecord{green}); err == nil {
		t.Fatal("GREEN without RED accepted")
	}
	green.Snapshot.Tests = "changed test"
	if err := ValidateTaskEvidence(p, c, []RunRecord{red, green}); err == nil {
		t.Fatal("changed test between RED/GREEN accepted")
	}
}
func TestRunGoJSONAndCancelledProcess(t *testing.T) {
	for _, mode := range []string{"go-pass", "go-fail"} {
		t.Run(mode, func(t *testing.T) {
			p := fixture(t)
			c, _ := LoadContract(p)
			policy := processPolicy(t, p, mode)
			policy.Suites[0].Format = "go-json"
			stage := "green"
			if mode == "go-fail" {
				stage = "red"
			}
			if _, err := Run(context.Background(), p, c, policy, 1, stage, t.TempDir()); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		p := fixture(t)
		c, _ := LoadContract(p)
		policy := processPolicy(t, p, "sleep")
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		r, err := Run(ctx, p, c, policy, 1, "green", t.TempDir())
		if err == nil || r.Success {
			t.Fatal("cancelled process accepted")
		}
	})
}
func TestEvidenceRejectsUnexecutedAndCrossProjectClaims(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	if err := ValidateTaskEvidence(p, c, nil); err == nil {
		t.Fatal("no execution accepted")
	}
	red, err := Run(context.Background(), p, c, processPolicy(t, p, "value"), 1, "red", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, "value.go", "package fixture\n// implemented\n")
	green, err := Run(context.Background(), p, c, processPolicy(t, p, "value"), 1, "green", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other := fixture(t)
	otherC, _ := LoadContract(other)
	if err := ValidateTaskEvidence(other, otherC, []RunRecord{red, green}); err == nil {
		t.Fatal("cross-project receipts accepted")
	}
	red.Snapshot.Change = "other"
	if err := ValidateTaskEvidence(p, c, []RunRecord{red, green}); err == nil {
		t.Fatal("cross-change receipts accepted")
	}
}
func TestEvidenceVerificationBindsCurrentSourceAndReport(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	report := "docs/changes/001-x/report.md"
	put(t, p.Root, report, "# Review\n```mulix-review\nreviewer: independent reviewer\nresult: approved\nblocking: []\n```\n")
	record, err := Run(context.Background(), p, c, processPolicy(t, p, "pass"), 0, "verify", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(p.Root, report))
	if err != nil {
		t.Fatal(err)
	}
	record.ReportDigest = Digest(data)
	if err := ValidateVerification(p, c, []RunRecord{record}, report); err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, "value.go", "package fixture\n// changed after verification\n")
	if err := ValidateVerification(p, c, []RunRecord{record}, report); err == nil {
		t.Fatal("stale source accepted")
	}
	put(t, p.Root, "value.go", "package fixture\n")
	put(t, p.Root, report, string(data)+"edited after verification\n")
	if err := ValidateVerification(p, c, []RunRecord{record}, report); err == nil {
		t.Fatal("modified report accepted")
	}
}
func TestEvidenceReviewRequiresNoBlockingFindings(t *testing.T) {
	p := fixture(t)
	report := "docs/changes/001-x/report.md"
	for _, text := range []string{"# all good\n", "```mulix-review\nreviewer: reviewer\nresult: approved\nblocking: [data loss]\n```\n", "```mulix-review\nreviewer: reviewer\nresult: pending\nblocking: []\n```\n"} {
		put(t, p.Root, report, text)
		if err := ValidateReview(p.Root, report); err == nil {
			t.Fatalf("review accepted: %s", text)
		}
	}
}
func TestRunNonCodeTaskUsesHonestCheck(t *testing.T) {
	p := fixture(t)
	put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "kind: code", "kind: verification\n    reason: verify an existing public behavior without implementation changes", 1))
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	check, err := Run(context.Background(), p, c, processPolicy(t, p, "pass"), 1, "check", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTaskEvidence(p, c, []RunRecord{check}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), p, c, processPolicy(t, p, "fail"), 1, "red", t.TempDir()); err == nil {
		t.Fatal("invented RED stage for a non-code task accepted")
	}
}
func TestRunRecordRoundTrip(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	r, err := Run(context.Background(), p, c, processPolicy(t, p, "pass"), 1, "green", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var restored RunRecord
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ID == "" || restored.Snapshot != r.Snapshot || restored.Suites[0].Command[0] == "" {
		t.Fatal("record lost execution identity")
	}
}
