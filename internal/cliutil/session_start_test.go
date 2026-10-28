package cliutil

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStart_OnlyExportsIdentityAndDoesNotBind(t *testing.T) {
	root, _ := sessionProject(t)
	envFile := filepath.Join(t.TempDir(), "host-env")
	payload, _ := json.Marshal(map[string]string{"hook_event_name": "SessionStart", "session_id": "A", "cwd": root})
	var stdout, stderr bytes.Buffer
	if err := runSessionStart(context.Background(), root, envFile, payload, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(envFile)
	if err != nil || !strings.Contains(string(data), "export MULIX_SESSION_ID=") {
		t.Fatalf("env=%s %v", data, err)
	}
	if strings.Contains(string(data), "MULIX_CHANGE") || strings.Contains(string(data), "runtime") {
		t.Fatal("cached a change", string(data))
	}
	if _, err := os.Stat(filepath.Join(root, ".mulix", "sessions")); !os.IsNotExist(err) {
		t.Fatal("startup created bindings", err)
	}
	active, err := os.ReadFile(filepath.Join(root, ".mulix", "active"))
	if err != nil || string(active) != "X\n" {
		t.Fatal("startup changed global")
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("stdout not JSON: %q %v", stdout.String(), err)
	}
}
func TestSessionStart_NoEnvFileHasExplicitParameterGuidance(t *testing.T) {
	root, _ := sessionProject(t)
	payload, _ := json.Marshal(map[string]string{"session_id": "A", "cwd": root})
	var stdout, stderr bytes.Buffer
	if err := runSessionStart(context.Background(), root, "", payload, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "--session-id") || !strings.Contains(stdout.String(), "A") {
		t.Fatal("missing explicit context guidance", stdout.String())
	}
}
