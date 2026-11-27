package cliutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
)

func newTrackerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tracker", Short: "Publish, synchronize and close bound Issues with explicit approval"}
	cmd.AddCommand(newTrackerPublishCmd(), newTrackerSnapshotCmd(false), newTrackerSnapshotCmd(true), newTrackerCloseCmd())
	return cmd
}

func newTrackerPublishCmd() *cobra.Command {
	var change, expect string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "publish [spec|tasks]", Short: "Preview or confirm creation of the spec Issue or the unpublished task batch",
		Long: "New spec publication requires specify; new task publication requires tasks and a verified spec snapshot. " +
			"Without a selector, use the current phase. Pending publications are reconciled separately and never POSTed again. " +
			"Run --dry-run, review the complete plan, then pass its token to --expect after obtaining authorization. A token pins content, not human identity or permission.",
		Args: cobra.MaximumNArgs(1), ValidArgs: []string{"spec", "tasks"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun == (expect != "") {
				return fmt.Errorf("use either --dry-run or --expect TOKEN from a reviewed dry run")
			}
			scope := ""
			if len(args) == 1 {
				scope = args[0]
				if scope != "spec" && scope != "tasks" {
					return fmt.Errorf("publish selector must be spec or tasks")
				}
			}
			return withTrackerChange(cmd, change, dryRun, func(root string, s flow.State) error {
				p, err := preparePublication(cmd.Context(), root, s, scope)
				if err != nil {
					return err
				}
				token, err := trackerToken("publish", p)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if dryRun {
					printTrackerTarget(out, p.Target, p.Change)
					for _, item := range p.Items {
						action := "Create"
						if p.Recovery {
							action = "Recover #" + item.Number + " (no POST)"
						}
						fmt.Fprintf(out, "%s %s at %s\n", action, tracker.SlotName(item.Slot), item.Intent.Path)
						if !p.Recovery {
							fmt.Fprintf(out, "Title: %s\n", item.Title)
						}
						showTrackerBody(out, "Approved body", item.Body)
					}
					fmt.Fprintf(out, "Token: %s\n", token)
					return nil
				}
				if token != expect {
					return fmt.Errorf("publication approval is stale; run tracker publish %s --dry-run again", p.Scope)
				}
				return executePublication(cmd.Context(), out, root, s, p, state.SaveTrackerCheckpoint)
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the complete publication plan and token without writing")
	cmd.Flags().StringVar(&expect, "expect", "", "confirm an exact publication plan using its dry-run token")
	cmd.Flags().StringVar(&change, "change", "", "change ID (defaults to effective session context)")
	return cmd
}

func withTrackerChange(cmd *cobra.Command, change string, readOnly bool, use func(string, flow.State) error) error {
	access := sessionctx.Mutate
	if readOnly {
		access = sessionctx.InspectReadOnly
	}
	return withRemoteChange(cmd, change, access, func(root string, s flow.State) error {
		if !readOnly && s.Archived {
			return fmt.Errorf("archived changes cannot be published or synchronized")
		}
		return use(root, s)
	})
}

func withRemoteChange(cmd *cobra.Command, change string, access sessionctx.Access, use func(string, flow.State) error) error {
	store, r, err := resolveCommandContext(cmd, change, access)
	if err != nil {
		return err
	}
	if err := requireChange(r); err != nil {
		return err
	}
	root := store.Root()
	check := func(s flow.State) error {
		if s.Change != r.Change || s.ArtifactLayout != flow.RuntimeIssuesLayout || s.Tracker == nil || s.Tracker.Target.Kind == "markdown" {
			return fmt.Errorf("tracker commands require a change with a pinned remote tracker")
		}
		return use(root, s)
	}
	if access == sessionctx.InspectReadOnly {
		return check(*r.State)
	}
	// Publication, snapshot writes, and closure share the owning change
	// lock. Do not reenter it through state.Update or a CLI child process.
	return lockfile.With(cmd.Context(), root, lockfile.ChangeKey(r.Change), func() error {
		if _, err := fileutil.ControlPath(root, layout.ChangeRuntimePath(r.Change, layout.StateFile), false); err != nil {
			return err
		}
		s, err := state.Load(root, r.Change)
		if err != nil {
			return err
		}
		return check(s)
	})
}

func trackerToken(operation string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte("mulix/tracker/v1/"+operation+"\x00"), data...))), nil
}

func printTrackerTarget(w io.Writer, target tracker.Target, change string) {
	fmt.Fprintf(w, "Tracker: %s %s/%s (project ID %s)\nChange: %s\n", target.Kind, target.Host, target.Project, target.ProjectID, change)
}

func showTrackerBody(w io.Writer, heading, body string) {
	fmt.Fprintln(w, heading+":")
	for line := range strings.SplitSeq(tracker.NormalizeBody(body), "\n") {
		// Remote text is data, not terminal control sequences or token lines.
		var safe strings.Builder
		safe.WriteString("  ")
		for _, r := range line {
			if r != '\t' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)) {
				safe.WriteString(strconv.QuoteRuneToASCII(r))
			} else {
				safe.WriteRune(r)
			}
		}
		fmt.Fprintln(w, safe.String())
	}
}

type publicationDraft struct {
	Slot     int
	Path     string
	Digest   string
	Markdown string `json:"-"`
}

type publicationItem struct {
	Slot   int
	Title  string
	Body   string `json:"-"`
	Intent tracker.IssueBinding
	ID     string
	Number string
	NodeID string
}

type publicationPlan struct {
	Change, Scope string
	Phase         flow.Phase
	Target        tracker.Target
	Previous      tracker.ChangeBinding
	Drafts        []publicationDraft
	Items         []publicationItem
	Recovery      bool
}

func readPublicationDrafts(root string, s flow.State, scope string) ([]publicationDraft, error) {
	p := s.QualityProject(root)
	var inputs []publicationDraft
	add := func(slot int, path string, md []byte) error {
		if !utf8.Valid(md) || strings.TrimSpace(string(md)) == "" {
			return fmt.Errorf("%s draft needs nonempty UTF-8 Markdown", tracker.SlotName(slot))
		}
		inputs = append(inputs, publicationDraft{slot, path, tracker.BodyDigest(string(md)), string(md)})
		return nil
	}
	spec, err := quality.ReadProjectFile(root, p.SpecPath)
	if err != nil {
		return nil, err
	}
	if err := add(0, p.SpecPath, spec); err != nil {
		return nil, err
	}
	if scope == "tasks" {
		docs, err := quality.ReadTaskDocuments(p)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			if err := add(doc.Number, doc.Path, doc.Markdown); err != nil {
				return nil, err
			}
		}
	}
	return inputs, nil
}

func publicationPrefix(b tracker.ChangeBinding, slot int) string {
	prefix := b.Marker(slot) + "\n\n"
	if slot != 0 {
		// Both providers render #N as an Issue link within the same project.
		// A native reference also works for self-hosted web URLs with prefixes.
		prefix += "Parent spec: #" + b.Issues[0].Number + "\n\n"
	}
	return prefix
}

func readApprovedDraft(b tracker.ChangeBinding, slot int, issue tracker.Issue) (string, error) {
	// Strip only the exact generated prefix, accepting either line ending.
	// Keep the raw original draft bytes for the accepted local baseline.
	draft := issue.Body
	for want := range strings.SplitSeq(strings.TrimSuffix(publicationPrefix(b, slot), "\n"), "\n") {
		line, rest, ok := strings.Cut(draft, "\n")
		if !ok || strings.TrimSuffix(line, "\r") != want {
			return "", fmt.Errorf("%s Issue is missing its approved publication prefix", tracker.SlotName(slot))
		}
		draft = rest
	}
	if tracker.BodyDigest(draft) != b.Issues[slot].DraftDigest {
		return "", fmt.Errorf("%s readback does not match the original draft digest", tracker.SlotName(slot))
	}
	return draft, nil
}

func preparePublication(ctx context.Context, root string, s flow.State, scope string) (publicationPlan, error) {
	if s.Archived {
		return publicationPlan{}, fmt.Errorf("archived changes cannot be published")
	}
	if scope == "" {
		switch s.Phase {
		case flow.PhaseSpecify:
			scope = "spec"
		case flow.PhaseTasks:
			scope = "tasks"
		default:
			return publicationPlan{}, fmt.Errorf("use an explicit spec or tasks selector to recover a pending publication outside its initial phase")
		}
	}
	b, c := *s.Tracker, tracker.Client{}
	var pending []int
	for slot, binding := range b.Issues {
		if (slot == 0) == (scope == "spec") && !binding.Published() {
			pending = append(pending, slot)
		}
	}
	slices.Sort(pending)
	if len(pending) == 0 && (scope == "spec" && s.Phase != flow.PhaseSpecify || scope == "tasks" && s.Phase != flow.PhaseTasks) {
		return publicationPlan{}, fmt.Errorf("initial spec publication requires specify; initial task publication requires tasks")
	}
	target, err := c.Probe(ctx, b.Target)
	if err != nil {
		return publicationPlan{}, err
	}
	p := publicationPlan{Change: s.Change, Scope: scope, Phase: s.Phase, Target: target, Previous: b, Recovery: len(pending) != 0}
	b.Target = target
	if p.Recovery {
		// Reconcile only already approved intents. Do not reinterpret a changed
		// or now-invalid task draft as a new authorized POST.
		for _, slot := range pending {
			issue, err := b.RecoverCreate(ctx, c, slot)
			if err != nil {
				return p, err
			}
			if _, err := readApprovedDraft(b, slot, issue); err != nil {
				return p, err
			}
			m := b.Issues[slot]
			p.Items = append(p.Items, publicationItem{Slot: slot, Body: issue.Body, Intent: m, ID: issue.ID, Number: issue.Number, NodeID: issue.NodeID})
		}
		return p, nil
	}
	p.Drafts, err = readPublicationDrafts(root, s, scope)
	if err != nil {
		return p, err
	}
	for slot, m := range b.Issues {
		if (slot == 0) == (scope == "spec") && !slices.ContainsFunc(p.Drafts, func(d publicationDraft) bool { return d.Slot == slot && d.Path == m.Path }) {
			return p, fmt.Errorf("%s no longer has its pinned draft path %s", tracker.SlotName(slot), m.Path)
		}
	}
	for _, d := range p.Drafts {
		if _, exists := b.Issues[d.Slot]; exists || (scope == "tasks" && d.Slot == 0) {
			if _, err := b.VerifySnapshot(ctx, c, d.Slot, d.Path, []byte(d.Markdown)); err != nil {
				return p, err
			}
			continue
		}
		if err := requireUnusedMarker(ctx, c, b, d.Slot); err != nil {
			return p, err
		}
		marker := b.Marker(d.Slot)
		for line := range strings.SplitSeq(tracker.NormalizeBody(d.Markdown), "\n") {
			if line == marker {
				return p, fmt.Errorf("%s draft already contains its reserved Issue marker", tracker.SlotName(d.Slot))
			}
		}
		body := publicationPrefix(b, d.Slot) + d.Markdown
		p.Items = append(p.Items, publicationItem{
			Slot: d.Slot, Title: "Mulix " + tracker.SlotName(d.Slot) + ": " + s.Change, Body: body,
			Intent: tracker.IssueBinding{Path: d.Path, DraftDigest: d.Digest, PublishDigest: tracker.BodyDigest(body)},
		})
	}
	if len(p.Items) == 0 {
		return p, fmt.Errorf("no unpublished %s; use tracker preview to inspect a bound Issue", scope)
	}
	return p, nil
}

func requireUnusedMarker(ctx context.Context, c tracker.Client, b tracker.ChangeBinding, slot int) error {
	found, err := c.Find(ctx, b.Target, b.Marker(slot))
	if err != nil {
		return err
	}
	if len(found) != 0 {
		return fmt.Errorf("%s already has %d remote marker matches without a pending intent; refusing creation", tracker.SlotName(slot), len(found))
	}
	return nil
}

func executePublication(ctx context.Context, w io.Writer, root string, s flow.State, p publicationPlan, checkpoint func(string, flow.State) error) error {
	b, c := s.Tracker, tracker.Client{}
	b.Target = p.Target
	if b.Issues == nil {
		b.Issues = make(map[int]tracker.IssueBinding)
	}
	for i, item := range p.Items {
		issue := tracker.Issue{ID: item.ID, Number: item.Number, Body: item.Body, NodeID: item.NodeID}
		if !p.Recovery {
			if i != 0 {
				if err := requireUnusedMarker(ctx, c, *b, item.Slot); err != nil {
					return err
				}
				parent := p.Drafts[0]
				if _, err := b.VerifySnapshot(ctx, c, 0, parent.Path, []byte(parent.Markdown)); err != nil {
					return err
				}
			}
			// Recheck local drafts after network reads and between batch POSTs.
			// A raw path change is a new plan, even if its body is identical.
			drafts, err := readPublicationDrafts(root, s, p.Scope)
			if err != nil {
				return err
			}
			if !slices.EqualFunc(p.Drafts, drafts, func(a, b publicationDraft) bool {
				return a.Slot == b.Slot && a.Path == b.Path && a.Digest == b.Digest
			}) {
				return fmt.Errorf("publication drafts changed; run tracker publish %s --dry-run again", p.Scope)
			}
			b.Issues[item.Slot] = item.Intent
			if err := checkpoint(root, s); err != nil {
				return fmt.Errorf("checkpointing project pin and publication intent before POST: %w", err)
			}
			created, err := c.Create(ctx, b.Target, item.Title, item.Body)
			if err == nil {
				intent := b.Issues[item.Slot]
				intent.Receipt = &created
				b.Issues[item.Slot] = intent
				if err := checkpoint(root, s); err != nil {
					return fmt.Errorf("could not checkpoint the positive %s create receipt for Issue #%s; inspect this Issue before recovery: %w", tracker.SlotName(item.Slot), created.Number, err)
				}
				issue, err = b.RecoverCreate(ctx, c, item.Slot)
			}
			if err != nil {
				return fmt.Errorf("%s publication is pending; use tracker publish %s --dry-run to reconcile without another POST: %w", tracker.SlotName(item.Slot), p.Scope, err)
			}
		}
		baseline, err := readApprovedDraft(*b, item.Slot, issue)
		if err != nil {
			return err
		}
		m := b.Issues[item.Slot]
		m.ID, m.Number, m.NodeID, m.Baseline, m.BodyDigest = issue.ID, issue.Number, issue.NodeID, baseline, tracker.BodyDigest(issue.Body)
		b.Issues[item.Slot] = m
		if err := checkpoint(root, s); err != nil {
			return fmt.Errorf("could not durably checkpoint the verified Issue mapping; inspect the binding before proceeding: %w", err)
		}
		fmt.Fprintf(w, "Bound %s to Issue #%s (ID %s); local draft left unchanged\n", tracker.SlotName(item.Slot), issue.Number, issue.ID)
	}
	if p.Recovery {
		fmt.Fprintln(w, "Reconciled pending publications; use a fresh dry run for any remaining drafts.")
	}
	return nil
}

// GitHub's bound Issue read re-probes the project's slug. GitLab reads by
// numeric project ID, so the workflow rechecks the pinned selector explicitly.
func checkWorkflowTarget(ctx context.Context, c tracker.Client, target tracker.Target) error {
	if target.ProjectID == "" {
		return fmt.Errorf("remote tracker project is not verified; start with tracker publish spec --dry-run")
	}
	if target.Kind == "gitlab" {
		_, err := c.Probe(ctx, target)
		return err
	}
	return nil
}

// checkTrackerSync is the same read-only check used by guard and by a
// transition holding the owning change lock. It cannot be overridden.
func checkTrackerSync(ctx context.Context, root string, s flow.State, event flow.Event) ([]verifiedTrackerSnapshot, error) {
	if s.ArtifactLayout != flow.RuntimeIssuesLayout {
		return nil, nil
	}
	var requireTasks bool
	switch event {
	case flow.EventSpecComplete, flow.EventClarifyComplete, flow.EventClarifySkipped, flow.EventDesignApproved:
	case flow.EventTasksComplete, flow.EventBuildComplete, flow.EventVerifyPass, flow.EventArchived:
		requireTasks = true
	default:
		// verify-fail returns to Build for local correction; it qualifies nothing.
		return nil, nil
	}
	if s.Tracker == nil {
		return nil, fmt.Errorf("native change is missing its pinned tracker")
	}
	if s.Tracker.Target.Kind == "markdown" {
		return nil, nil
	}
	return verifyTrackerSnapshots(ctx, root, s, requireTasks)
}

// recheckTrackerInputs compares point-in-time local observations, not an
// editor or filesystem transaction. It performs no additional remote reads.
func recheckTrackerInputs(root string, s flow.State, verified []verifiedTrackerSnapshot) error {
	if len(verified) == 0 {
		return nil
	}
	spec, err := quality.ReadProjectFile(root, s.SpecPath)
	if err != nil {
		return err
	}
	docs := []quality.TaskDocument{{Path: s.SpecPath, Markdown: spec}}
	if len(verified) > 1 {
		tasks, err := quality.ReadTaskSnapshotInputs(s.QualityProject(root))
		if err != nil {
			return err
		}
		docs = append(docs, tasks...)
	}
	if len(docs) != len(verified) {
		return fmt.Errorf("local snapshot set changed during tracker check; retry")
	}
	for i, doc := range docs {
		old := verified[i].doc
		if doc.Number != old.Number || doc.Path != old.Path || !bytes.Equal(doc.Markdown, old.Markdown) {
			return fmt.Errorf("local snapshot %s changed during tracker check; retry", old.Path)
		}
	}
	return nil
}

type verifiedTrackerSnapshot struct {
	doc   quality.TaskDocument // slot 0 represents the spec
	issue tracker.Issue
}

// verifyTrackerSnapshots compares the complete owning path/Issue set without
// interpreting local acceptance or suite policy. Callers check those separately.
func verifyTrackerSnapshots(ctx context.Context, root string, s flow.State, requireTasks bool) ([]verifiedTrackerSnapshot, error) {
	b, c := s.Tracker, tracker.Client{}
	if err := checkWorkflowTarget(ctx, c, b.Target); err != nil {
		return nil, err
	}
	spec, err := quality.ReadProjectFile(root, s.SpecPath)
	if err != nil {
		return nil, err
	}
	docs := []quality.TaskDocument{{Path: s.SpecPath, Markdown: spec}}
	if requireTasks {
		tasks, err := quality.ReadTaskSnapshotInputs(s.QualityProject(root))
		if err != nil {
			return nil, err
		}
		docs = append(docs, tasks...)
		if len(b.Issues) != len(docs) {
			return nil, fmt.Errorf("bound Issue slots must match the spec and every task exactly; restore published paths or use tracker publish tasks --dry-run for unpublished drafts")
		}
	}
	verified := make([]verifiedTrackerSnapshot, 0, len(docs))
	for _, doc := range docs {
		issue, err := b.VerifySnapshot(ctx, c, doc.Number, doc.Path, doc.Markdown)
		if err != nil {
			return nil, err
		}
		verified = append(verified, verifiedTrackerSnapshot{doc: doc, issue: issue})
	}
	return verified, nil
}

func newTrackerCloseCmd() *cobra.Command {
	var change, expect string
	var dryRun bool
	cmd := &cobra.Command{
		Use: "close <spec|task-N>", Short: "Separately authorize closing one eligible bound Issue",
		Long: "Task close requires active Build, a current GREEN/CHECK receipt and its matching ledger line. " +
			"Spec close requires an already archived --change. Run --dry-run, obtain authorization, then pass its token to --expect. " +
			"Closing an Issue does not run tests, authenticate review, or verify integration.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slot, err := trackerSlot(args[0])
			if err != nil {
				return err
			}
			if dryRun == (expect != "") {
				return fmt.Errorf("use either --dry-run or --expect TOKEN from an authorized close dry run")
			}
			access := sessionctx.Mutate
			if slot == 0 {
				if change == "" {
					return fmt.Errorf("spec Issue closure requires explicit --change ID after archive")
				}
				access = sessionctx.CloseArchivedSpec
			}
			if dryRun {
				access = sessionctx.InspectReadOnly
			}
			return withRemoteChange(cmd, change, access, func(root string, s flow.State) error {
				view, err := prepareClosure(cmd.Context(), root, s, slot)
				if err != nil {
					return err
				}
				// The digest binds the normalized body version; pass the full
				// verified Issue, unmodified, to the provider for its last read.
				intent := view
				intent.Issue.Body = ""
				token, err := trackerToken("close", intent)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if dryRun {
					printTrackerTarget(out, view.Target, view.Change)
					fmt.Fprintf(out, "Close %s Issue #%s (ID %s) for %s\nBody SHA-256: %s\n", args[0], view.Issue.Number, view.Issue.ID, view.Path, view.RemoteDigest)
					if slot == 0 {
						fmt.Fprintln(out, "Eligibility: change already archived locally.")
					} else {
						fmt.Fprintf(out, "Eligibility: current %s receipt %s and matching Build ledger entry.\n", view.Stage, view.Receipt)
					}
					fmt.Fprintf(out, "Token: %s\n", token)
					return nil
				}
				if token != expect {
					return fmt.Errorf("closure approval is stale; review a fresh tracker close %s --dry-run", args[0])
				}
				if err := (tracker.Client{}).Close(cmd.Context(), view.Target, view.Issue); err != nil {
					return fmt.Errorf("closure not confirmed for %s; local change state was not modified: %w", args[0], err)
				}
				fmt.Fprintf(out, "Closed %s Issue #%s; local change state unchanged\n", args[0], view.Issue.Number)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the bound Issue, eligibility and approval token without writing")
	cmd.Flags().StringVar(&expect, "expect", "", "confirm the exact authorized closure with its dry-run token")
	cmd.Flags().StringVar(&change, "change", "", "required for spec; tasks default to the effective change")
	return cmd
}

type closureView struct {
	Change, Key               string
	Phase                     flow.Phase
	Target                    tracker.Target
	Slot                      int
	Path                      string
	Issue                     tracker.Issue
	LocalDigest, RemoteDigest string
	Stage, Receipt            string
}

func prepareClosure(ctx context.Context, root string, s flow.State, slot int) (closureView, error) {
	if slot == 0 && (s.Phase != flow.PhaseArchive || !s.Archived) {
		return closureView{}, fmt.Errorf("spec Issue closure requires an already archived change")
	}
	if slot != 0 && (s.Phase != flow.PhaseBuild || s.Archived) {
		return closureView{}, fmt.Errorf("task Issue closure requires an active Build phase")
	}
	b := s.Tracker
	if !b.Issues[slot].Published() {
		return closureView{}, fmt.Errorf("%s has no verified Issue binding", tracker.SlotName(slot))
	}
	// Task receipts bind the entire native contract, so a task close must
	// recheck the parent spec and every task, not merely the Issue it writes.
	verified, err := verifyTrackerSnapshots(ctx, root, s, slot != 0)
	if err != nil {
		return closureView{}, err
	}
	i := slices.IndexFunc(verified, func(snap verifiedTrackerSnapshot) bool { return snap.doc.Number == slot })
	if i < 0 {
		return closureView{}, fmt.Errorf("%s has no current snapshot", tracker.SlotName(slot))
	}
	selected := verified[i]
	v := closureView{Change: s.Change, Key: b.RemoteKey, Phase: s.Phase, Target: b.Target, Slot: slot,
		Path: selected.doc.Path, Issue: selected.issue, LocalDigest: tracker.BodyDigest(string(selected.doc.Markdown)), RemoteDigest: b.Issues[slot].BodyDigest}
	if slot != 0 {
		r, err := guard.ValidateTaskClosure(root, s, slot)
		if err != nil {
			return closureView{}, err
		}
		// The receipt must attest to the exact paths and raw bytes that
		// were compared, even if local files changed during provider reads.
		for _, snap := range verified {
			doc := snap.doc
			if !slices.Contains(r.ArtifactInputs, quality.ArtifactInput{Path: doc.Path, SHA256: quality.Digest(doc.Markdown)}) {
				return closureView{}, fmt.Errorf("task receipt does not attest to the verified %s snapshot", tracker.SlotName(doc.Number))
			}
		}
		v.Stage, v.Receipt = r.Stage, r.ID
	}
	return v, nil
}

func newTrackerSnapshotCmd(apply bool) *cobra.Command {
	var change, expect string
	var backup bool
	cmd := &cobra.Command{
		Use: "preview <spec|task-N>", Short: "Read the baseline, local and remote body for one bound Issue without writing",
		Long: "Preview shows the last accepted local baseline, current local Markdown, and current remote Issue body. " +
			"To replace conflicting local edits, preview with --backup-local and use its token with the same flag on apply.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slot, err := trackerSlot(args[0])
			if err != nil {
				return err
			}
			if apply && expect == "" {
				return fmt.Errorf("apply requires --expect TOKEN from tracker preview")
			}
			return withTrackerChange(cmd, change, !apply, func(root string, s flow.State) error {
				v, err := readSnapshotView(cmd.Context(), root, s, slot, backup)
				if err != nil {
					return err
				}
				token, err := trackerToken("snapshot", v)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if !apply {
					v.preview(out, token)
					return nil
				}
				if token != expect {
					return fmt.Errorf("snapshot approval is stale; run tracker preview %s again", tracker.SlotName(slot))
				}
				return v.apply(out, root, s, os.Rename)
			})
		},
	}
	if apply {
		cmd.Use, cmd.Short = "apply <spec|task-N>", "Apply an approved remote Issue body to its one local snapshot"
		cmd.Long = "Recheck the exact preview under the change lock before writing. Local edits are protected by default; resolving a conflict requires the --backup-local preview token and --backup-local on apply. No remote writes."
		cmd.Flags().StringVar(&expect, "expect", "", "required token from tracker preview")
	}
	cmd.Flags().BoolVar(&backup, "backup-local", false, "allow replacement of conflicting local edits only after a durable local backup")
	cmd.Flags().StringVar(&change, "change", "", "change ID (defaults to effective session context)")
	return cmd
}

func trackerSlot(selector string) (int, error) {
	if selector == "spec" {
		return 0, nil
	}
	digits, ok := strings.CutPrefix(selector, "task-")
	if n, err := strconv.Atoi(digits); ok && err == nil && n > 0 && strconv.Itoa(n) == digits {
		return n, nil
	}
	return 0, fmt.Errorf("select spec or task-N, e.g. task-1")
}

type snapshotView struct {
	Change, Key string
	Phase       flow.Phase
	Target      tracker.Target
	Slot        int
	Binding     tracker.IssueBinding
	LocalDigest string
	NextDigest  string
	Backup      bool
	local       string
	remote      string
}

func readSnapshotView(ctx context.Context, root string, s flow.State, slot int, backup bool) (snapshotView, error) {
	issue, err := s.Tracker.ReadBoundIssue(ctx, tracker.Client{}, slot)
	if err != nil {
		return snapshotView{}, err
	}
	m := s.Tracker.Issues[slot]
	local, err := quality.ReadProjectFile(root, m.Path)
	if err != nil {
		return snapshotView{}, err
	}
	return snapshotView{
		Change: s.Change, Key: s.Tracker.RemoteKey, Phase: s.Phase, Target: s.Tracker.Target,
		Slot: slot, Binding: m, LocalDigest: tracker.BodyDigest(string(local)), NextDigest: tracker.BodyDigest(issue.Body),
		Backup: backup, local: string(local), remote: issue.Body,
	}, nil
}

func (v snapshotView) conflict() bool {
	return v.LocalDigest != tracker.BodyDigest(v.Binding.Baseline) && v.LocalDigest != v.NextDigest
}

func (v snapshotView) unchanged() bool {
	return v.LocalDigest == tracker.BodyDigest(v.Binding.Baseline) && v.NextDigest == v.Binding.BodyDigest
}

func (v snapshotView) preview(out io.Writer, token string) {
	printTrackerTarget(out, v.Target, v.Change)
	fmt.Fprintf(out, "%s: Issue #%s (ID %s) -> %s\n", tracker.SlotName(v.Slot), v.Binding.Number, v.Binding.ID, v.Binding.Path)
	showTrackerBody(out, "Last accepted local baseline", v.Binding.Baseline)
	showTrackerBody(out, "Current local body", v.local)
	showTrackerBody(out, "Current remote body", v.remote)
	switch {
	case v.unchanged():
		fmt.Fprintln(out, "The bound bodies are unchanged.")
	case v.conflict() && !v.Backup:
		fmt.Fprintln(out, "Local changes protected. To accept the remote body, preview again with --backup-local; review all three bodies before applying.")
	case v.conflict():
		fmt.Fprintln(out, "Apply with --backup-local to save an exact local backup before replacing the snapshot.")
	case v.LocalDigest == v.NextDigest:
		fmt.Fprintln(out, "Local and remote bodies already match; apply will only reconcile snapshot metadata.")
	}
	fmt.Fprintf(out, "Token: %s\n", token)
}

func (v snapshotView) apply(out io.Writer, root string, s flow.State, move func(string, string) error) error {
	if v.unchanged() {
		fmt.Fprintln(out, "The bound bodies are unchanged.")
		return nil
	}
	if v.conflict() && !v.Backup {
		return fmt.Errorf("local changes protected; use a fresh tracker preview %s --backup-local to approve a backed-up replacement", tracker.SlotName(v.Slot))
	}
	if v.LocalDigest != v.NextDigest {
		if err := v.replaceLocal(out, root, move); err != nil {
			return err
		}
	}
	// Interruption after installing the snapshot can leave a file equal to the
	// bound Issue but old metadata. Only a fresh preview/readback of equality
	// may reconcile metadata without replacing a file again.
	m := v.Binding
	m.Baseline, m.BodyDigest = v.remote, v.NextDigest
	s.Tracker.Issues[v.Slot] = m
	if err := state.Save(root, s); err != nil {
		return fmt.Errorf("snapshot matches the reviewed Issue body, but metadata save failed; preview again to reconcile: %w", err)
	}
	fmt.Fprintf(out, "Synchronized %s from Issue #%s\n", m.Path, m.Number)
	return nil
}

// replaceLocal never overwrites a file at the active path. The old inode
// remains reachable for editor writes, separate from the durable exact-byte
// copy. A new editor save at the active path wins.
func (v snapshotView) replaceLocal(out io.Writer, root string, move func(string, string) error) error {
	full, err := fileutil.ControlPath(root, v.Binding.Path, false)
	if err != nil {
		return err
	}
	staged, err := stageTrackerBody(full, []byte(v.remote))
	if err != nil {
		return err
	}
	defer os.Remove(staged)

	backupDir := layout.ChangeRuntimePath(v.Change, "tracker-backups")
	probe, err := fileutil.ControlPath(root, backupDir+"/.probe", true)
	if err != nil {
		return err
	}
	placeholder, err := os.CreateTemp(filepath.Dir(probe), tracker.SlotName(v.Slot)+"-*.md")
	if err != nil {
		return err
	}
	backup := placeholder.Name()
	backupRel := backupDir + "/" + filepath.Base(backup)
	reserved, statErr := placeholder.Stat()
	closeErr := placeholder.Close()
	if statErr != nil || closeErr != nil {
		_ = os.Remove(backup)
		return fmt.Errorf("could not reserve local backup %s", backupRel)
	}
	if err := move(full, backup); err != nil {
		// A failed/uncertain rename is not permission to remove a displaced
		// file. Clean up only our empty, still-identical placeholder.
		if info, statErr := os.Lstat(backup); statErr == nil && info.Size() == 0 && os.SameFile(info, reserved) {
			_ = os.Remove(backup)
		}
		return fmt.Errorf("could not displace local file; inspect %s and %s before retrying: %w", v.Binding.Path, backupRel, err)
	}

	info, err := os.Lstat(backup)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("displaced path is no longer a regular file; inspect %s (nothing installed)", backupRel)
	}
	restore := func(cause error) error {
		if err := os.Link(backup, full); err == nil {
			return fmt.Errorf("%w; restored local path without overwriting and kept %s", cause, backupRel)
		}
		return fmt.Errorf("%w; did not overwrite the active path; displaced file retained at %s", cause, backupRel)
	}
	displaced, err := os.ReadFile(backup)
	if err != nil {
		return restore(fmt.Errorf("could not inspect displaced local file: %w", err))
	}
	if string(displaced) != v.local {
		return restore(fmt.Errorf("local body changed during apply; preview again"))
	}
	if err := os.Chmod(staged, info.Mode().Perm()); err != nil {
		return restore(err)
	}
	// A held-open editor descriptor can still change the displaced inode.
	// Copy the bytes we actually verified into a unique, private directory
	// before installing anything, without replacing any previous backup.
	approvedDir, err := os.MkdirTemp(filepath.Dir(probe), tracker.SlotName(v.Slot)+"-approved-")
	if err != nil {
		return restore(fmt.Errorf("could not reserve an exact approved backup: %w", err))
	}
	approvedRel := backupDir + "/" + filepath.Base(approvedDir) + "/local.md"
	approvedPath, err := fileutil.ControlPath(root, approvedRel, false)
	if err == nil {
		err = fileutil.WriteFileDurable(root, approvedPath, displaced, 0600)
	}
	if err != nil {
		_ = os.Remove(approvedDir) // empty only; retain any already written bytes
		return restore(fmt.Errorf("could not durably write exact backup at %s; no snapshot installed: %w", approvedRel, err))
	}
	fmt.Fprintf(out, "Kept exact approved backup at %s\n", approvedRel)
	// Link installs the prepared inode only if the name is unoccupied; unlike
	// rename it cannot trample an editor's recreated path.
	if err := os.Link(staged, full); err != nil {
		return restore(fmt.Errorf("could not install snapshot without overwriting a local file: %w", err))
	}
	current, err := quality.ReadProjectFile(root, v.Binding.Path)
	if err != nil || string(current) != v.remote {
		return fmt.Errorf("local path changed after snapshot installation; no metadata saved; displaced file retained at %s", backupRel)
	}
	fmt.Fprintf(out, "Retained displaced editing file at %s\n", backupRel)
	return nil
}

func stageTrackerBody(target string, body []byte) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".mulix-snapshot-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := fileutil.WriteFile(path, body, 0600); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
