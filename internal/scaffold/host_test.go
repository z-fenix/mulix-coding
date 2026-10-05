package scaffold

import (
	"reflect"
	"testing"
)

func TestParseHosts(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []Host
		wantErr bool
	}{
		{name: "empty selects nothing", values: nil, want: nil},
		{name: "single host", values: []string{"claude"}, want: []Host{HostClaude}},
		{name: "dsh", values: []string{"dsh"}, want: []Host{HostDSH}},
		{name: "comma separated", values: []string{"claude,dsh"}, want: []Host{HostClaude, HostDSH}},
		{name: "repeated flag", values: []string{"dsh", "claude"}, want: []Host{HostDSH, HostClaude}},
		{name: "duplicates collapse", values: []string{"claude,dsh", "dsh"}, want: []Host{HostClaude, HostDSH}},
		{name: "case and space tolerant", values: []string{" DSH "}, want: []Host{HostDSH}},
		{name: "empty segments ignored", values: []string{",claude,,"}, want: []Host{HostClaude}},
		{name: "unknown host", values: []string{"cursor"}, wantErr: true},
		{name: "unknown host in list", values: []string{"claude,codex"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHosts(tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %v, got hosts %v", tt.values, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHosts(%v): %v", tt.values, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseHosts(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

func TestNormalizeHosts(t *testing.T) {
	t.Run("zero value defaults to claude", func(t *testing.T) {
		got, err := normalizeHosts(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []Host{HostClaude}) {
			t.Fatalf("normalizeHosts(nil) = %v, want [claude]", got)
		}
	})
	t.Run("empty slice defaults to claude", func(t *testing.T) {
		got, err := normalizeHosts([]Host{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []Host{HostClaude}) {
			t.Fatalf("normalizeHosts(empty) = %v, want [claude]", got)
		}
	})
	t.Run("explicit hosts pass through", func(t *testing.T) {
		got, err := normalizeHosts([]Host{HostDSH})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []Host{HostDSH}) {
			t.Fatalf("normalizeHosts([dsh]) = %v, want [dsh]", got)
		}
	})
	t.Run("unknown host rejected", func(t *testing.T) {
		if _, err := normalizeHosts([]Host{"codex"}); err == nil {
			t.Fatal("expected an error for an unknown host")
		}
	})
}

func TestHostSkillsDir(t *testing.T) {
	if got := HostClaude.skillsDir(); got != ".claude/skills" {
		t.Fatalf("claude skillsDir = %q", got)
	}
	if got := HostDSH.skillsDir(); got != ".dsh/skills" {
		t.Fatalf("dsh skillsDir = %q", got)
	}
}

func TestHostCapabilities(t *testing.T) {
	if !HostClaude.installsHook() || HostDSH.installsHook() {
		t.Fatal("only Claude Code installs the PreToolUse hook")
	}
	if HostClaude.writesBootstrap() || !HostDSH.writesBootstrap() {
		t.Fatal("only DSH writes the AGENTS.md bootstrap")
	}
}
