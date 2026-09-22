package scaffold

import (
	"reflect"
	"strings"
	"testing"
)

func lines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func TestMerge3_NoChangesYieldsBase(t *testing.T) {
	base := lines("a\nb\nc")
	got, conflicts := merge3(base, base, base)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("expected base unchanged, got %v", got)
	}
}

func TestMerge3_LocalUnchangedTakesIncoming(t *testing.T) {
	base := lines("a\nb\nc")
	incoming := lines("a\nB\nc") // mulix updated line b
	got, conflicts := merge3(base, incoming, base)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	if !reflect.DeepEqual(got, incoming) {
		t.Fatalf("expected incoming to win, got %v", got)
	}
}

func TestMerge3_IncomingUnchangedKeepsLocal(t *testing.T) {
	base := lines("a\nb\nc")
	local := lines("a\nb\nc\nlocal extra") // user added a line
	got, conflicts := merge3(base, base, local)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	if !reflect.DeepEqual(got, local) {
		t.Fatalf("expected local to win, got %v", got)
	}
}

func TestMerge3_DisjointChangesBothApplied(t *testing.T) {
	base := lines("a\nb\nc\nd\ne")
	incoming := lines("A\nb\nc\nd\ne") // changed line 1
	local := lines("a\nb\nc\nd\nE")    // changed line 5
	got, conflicts := merge3(base, incoming, local)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	want := lines("A\nb\nc\nd\nE")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected both changes merged, got %v", got)
	}
}

func TestMerge3_SameChangeOnBothSidesTakenOnce(t *testing.T) {
	base := lines("a\nb\nc")
	both := lines("a\nX\nc")
	got, conflicts := merge3(base, both, both)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	if !reflect.DeepEqual(got, both) {
		t.Fatalf("expected shared change once, got %v", got)
	}
}

func TestMerge3_OverlappingChangesConflict(t *testing.T) {
	base := lines("a\nb\nc")
	incoming := lines("a\nincoming\nc")
	local := lines("a\nlocal\nc")
	got, conflicts := merge3(base, incoming, local)
	if conflicts != 1 {
		t.Fatalf("expected 1 conflict, got %d", conflicts)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"<<<<<<<", "incoming", "=======", "local", ">>>>>>>"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected conflict markers containing %q, got:\n%s", want, joined)
		}
	}
}

func TestMerge3_AdjacentChangesConflict(t *testing.T) {
	// Git treats directly adjacent changed lines as a conflict; so do we,
	// to keep silent merges conservative.
	base := lines("a\nb\nc\nd")
	incoming := lines("a\nB\nc\nd") // changed base line 2
	local := lines("a\nb\nC\nd")    // changed base line 3
	_, conflicts := merge3(base, incoming, local)
	if conflicts != 1 {
		t.Fatalf("expected adjacent changes to conflict (1), got %d", conflicts)
	}
}

func TestMerge3_InsertionsAtSameSpotConflict(t *testing.T) {
	base := lines("a")
	incoming := lines("a\nfrom incoming")
	local := lines("a\nfrom local")
	got, conflicts := merge3(base, incoming, local)
	if conflicts != 1 {
		t.Fatalf("expected 1 conflict, got %d", conflicts)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "from incoming") || !strings.Contains(joined, "from local") {
		t.Fatalf("expected both sides in conflict block, got:\n%s", joined)
	}
}

func TestMerge3_DeletionVsEditConflicts(t *testing.T) {
	base := lines("a\nb\nc")
	incoming := lines("a\nc") // mulix deleted line b
	local := lines("a\nB\nc") // user edited line b
	got, conflicts := merge3(base, incoming, local)
	if conflicts != 1 {
		t.Fatalf("expected 1 conflict, got %d", conflicts)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "B") {
		t.Fatalf("expected user's edit in conflict block, got:\n%s", joined)
	}
}

func TestMerge3_NonAdjacentInsertionsMerge(t *testing.T) {
	base := lines("a\nb\nc\nd\ne")
	incoming := lines("from mulix\na\nb\nc\nd\ne") // inserted at top
	local := lines("a\nb\nc\nd\ne\nfrom local")    // inserted at bottom
	got, conflicts := merge3(base, incoming, local)
	if conflicts != 0 {
		t.Fatalf("expected 0 conflicts, got %d", conflicts)
	}
	want := lines("from mulix\na\nb\nc\nd\ne\nfrom local")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected both insertions, got %v", got)
	}
}
