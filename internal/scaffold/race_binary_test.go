package scaffold

import (
	"debug/buildinfo"
	"testing"
)

func TestSessionScriptBinaryInheritsRaceInstrumentation(t *testing.T) {
	info, err := buildinfo.ReadFile(sessionTestBin)
	if err != nil {
		t.Fatal(err)
	}
	instrumented := false
	for _, setting := range info.Settings {
		if setting.Key == "-race" && setting.Value == "true" {
			instrumented = true
		}
	}
	if instrumented != scriptRaceEnabled {
		t.Fatalf("separate CLI race instrumentation = %v, parent test binary = %v", instrumented, scriptRaceEnabled)
	}
}
