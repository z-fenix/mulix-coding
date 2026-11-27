package tracker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/tracker"
)

func init() {
	if os.Getenv("MULIX_TRACKER_TEST_HELPER") != "1" {
		return
	}
	args := strings.Join(os.Args[1:], " ")
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	f, err := os.OpenFile(os.Getenv("MULIX_TRACKER_TEST_LOG"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(2)
	}
	fmt.Fprintln(f, name+" "+args)
	f.Close()
	if filename := os.Getenv("MULIX_TRACKER_TEST_ROUTES"); filename != "" {
		data, err := os.ReadFile(filename)
		var routes map[string]issueRoute
		if err != nil || json.Unmarshal(data, &routes) != nil {
			os.Exit(2)
		}
		route, ok := routes[name+" "+args]
		if !ok || route.Fail {
			fmt.Fprintln(os.Stderr, "authentication or transport failure: fake-auth-secret")
			os.Exit(1)
		}
		if route.Request != nil {
			var request map[string]any
			if json.NewDecoder(os.Stdin).Decode(&request) != nil || !reflect.DeepEqual(request, route.Request) {
				os.Exit(2)
			}
		}
		fmt.Print(route.Response)
		os.Exit(0)
	}
	switch name + " " + args {
	case "gh api repos/team/repo --hostname github.example.test":
		fmt.Print(`{"id":42,"full_name":"team/repo"}`)
	case "glab api projects/team%2Frepo --hostname gitlab.com":
		fmt.Print(`{"id":74,"path_with_namespace":"team/repo"}`)
	case "glab api projects/team%2Fsub%2Frepo --hostname git.example.test":
		fmt.Print(`{"id":73,"path_with_namespace":"team/sub/repo"}`)
	default:
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeProviderCLIs(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"gh", "glab"} {
		if runtime.GOOS == "windows" {
			n += ".exe"
		}
		in, err := os.Open(source)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.OpenFile(filepath.Join(bin, n), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			out.Close()
			t.Fatal(err)
		}
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("MULIX_TRACKER_TEST_HELPER", "1")
	t.Setenv("MULIX_TRACKER_TEST_LOG", log)
	t.Setenv("MULIX_TRACKER_TEST_ROUTES", "")
	return log
}

func TestTrackerProbePinnedTarget(t *testing.T) {
	log := fakeProviderCLIs(t)
	cases := []struct {
		target tracker.Target
		id     string
	}{
		{tracker.Target{Kind: "markdown"}, ""},
		{tracker.Target{Kind: "github", Host: "github.example.test", Project: "team/repo"}, "42"},
		{tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo"}, "74"},
		{tracker.Target{Kind: "gitlab", Host: "git.example.test", Project: "team/sub/repo"}, "73"},
	}
	for _, tc := range cases {
		got, err := (tracker.Client{}).Probe(context.Background(), tc.target)
		if err != nil || got.ProjectID != tc.id {
			t.Errorf("Probe(%+v) = %+v, %v", tc.target, got, err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.TrimSpace(string(data)), "\n") != 2 {
		t.Fatalf("unexpected number of network calls: %s", data)
	}
	if _, err := (tracker.Client{}).Probe(context.Background(), tracker.Target{Kind: "github", Host: "github.example.test", Project: "team/repo", ProjectID: "999"}); err == nil {
		t.Fatal("changed project ID accepted")
	}
}

func TestTrackerLoadRejectsLinkedConfig(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "tracker.json"), []byte(`{"kind":"markdown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".mulix")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v: %s", err, out)
		}
	} else if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tracker.LoadConfig(root); err == nil {
		t.Fatal("LoadConfig followed a symlink outside the project")
	}
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "markdown"}); err == nil {
		t.Fatal("SaveConfig followed a symlink outside the project")
	}
}

func TestTrackerLoadConfigDistinguishesInvalidAndMissing(t *testing.T) {
	root := t.TempDir()
	if _, found, err := tracker.LoadConfig(root); found || err != nil {
		t.Fatalf("missing: found %v err %v", found, err)
	}
	if err := os.Mkdir(filepath.Join(root, ".mulix"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mulix/tracker.json"), []byte(`{"kind":"github","host":"https://token@host","project":"a/b"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := tracker.LoadConfig(root); !found || err == nil {
		t.Fatalf("unsafe target accepted: found %v err %v", found, err)
	}
}
