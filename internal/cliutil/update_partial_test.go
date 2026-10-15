package cliutil

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/scaffold"
)

func TestUpdateReview_PrintEveryPartialOutcome(t *testing.T) {
	// Deserialize the result contract to keep RED a missing-output failure,
	// rather than a compile error before the Partial result field exists.
	var result scaffold.UpdateResult
	if err := json.Unmarshal([]byte(`{"Partial":["one (local written; failed: vendor baseline)","two","three","four","five","six","seven"]}`), &result); err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() { os.Stdout = original }()
	printUpdateResult(result)
	w.Close()
	os.Stdout = original
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "7 partial") || !strings.Contains(string(out), "local written; failed: vendor baseline") || !strings.Contains(string(out), "- partial: seven") {
		t.Fatalf("partial paths/stages omitted: %s", out)
	}
}
