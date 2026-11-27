package tracker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// IssueBinding starts as a durable intent. ID and Number remain empty until a
// matching Issue has been read back. The original approved digests stay fixed
// even when the last verified local/remote snapshot pair advances.
// Baseline is the last accepted local Markdown, for a three-way preview.
type IssueBinding struct {
	Path          string         `yaml:"path"`
	DraftDigest   string         `yaml:"draft_digest"`
	PublishDigest string         `yaml:"publish_digest"`
	Receipt       *CreateReceipt `yaml:"receipt,omitempty"`
	ID            string         `yaml:"id,omitempty"`
	Number        string         `yaml:"number,omitempty"`
	NodeID        string         `yaml:"node_id,omitempty"`
	Baseline      string         `yaml:"baseline,omitempty"`
	BodyDigest    string         `yaml:"body_digest,omitempty"`
}

func (b IssueBinding) Published() bool { return b.ID != "" && b.Number != "" }

// SlotName and Marker name scheme: zero is the spec, positive numbers are tasks.
func SlotName(slot int) string {
	if slot == 0 {
		return "spec"
	}
	return "task-" + strconv.Itoa(slot)
}

func (b ChangeBinding) Marker(slot int) string {
	return "<!-- mulix:issue:" + b.RemoteKey + ":" + SlotName(slot) + " -->"
}

// NormalizeBody changes only CRLF to LF; do not trim blanks, Markdown or
// generated marker/link lines. Persist raw snapshots and normalize on compare.
func NormalizeBody(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func BodyDigest(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(NormalizeBody(body))))
}

func (b ChangeBinding) ReadBoundIssue(ctx context.Context, c Client, slot int) (Issue, error) {
	m, ok := b.Issues[slot]
	if !ok || !m.Published() {
		return Issue{}, fmt.Errorf("%s is not bound to a verified Issue; finish tracker publish first", SlotName(slot))
	}
	issue, err := c.Get(ctx, b.Target, m.Number)
	if err != nil {
		return Issue{}, err
	}
	if issue.ID != m.ID || issue.NodeID != m.NodeID || markerCount(issue.Body, b.Marker(slot)) != 1 {
		return Issue{}, fmt.Errorf("%s Issue no longer has the bound identity and marker", SlotName(slot))
	}
	return issue, nil
}

// VerifySnapshot checks the two accepted body versions and their immutable
// mapping. It is read-only and independent of the caller's phase policy. The
// caller supplies Markdown read from the slot's owning path.
func (b ChangeBinding) VerifySnapshot(ctx context.Context, c Client, slot int, path string, markdown []byte) (Issue, error) {
	m := b.Issues[slot]
	if m.Path != path {
		return Issue{}, fmt.Errorf("%s is unpublished or its local path no longer matches the binding", SlotName(slot))
	}
	issue, err := b.ReadBoundIssue(ctx, c, slot)
	if err != nil {
		return Issue{}, err
	}
	if BodyDigest(string(markdown)) != BodyDigest(m.Baseline) || BodyDigest(issue.Body) != m.BodyDigest {
		return Issue{}, fmt.Errorf("%s snapshot has drifted; use mulix tracker preview %s", SlotName(slot), SlotName(slot))
	}
	return issue, nil
}

// RecoverCreate reads back a unique marker match, then checks the exact body
// approved before the POST. A listing alone is not enough to adopt a mapping.
// If the create returned an identity, its durable receipt must also match.
func (b ChangeBinding) RecoverCreate(ctx context.Context, c Client, slot int) (Issue, error) {
	m, ok := b.Issues[slot]
	if !ok || m.Published() {
		return Issue{}, fmt.Errorf("%s has no pending publication to reconcile", SlotName(slot))
	}
	marker := b.Marker(slot)
	found, err := c.Find(ctx, b.Target, marker)
	if err != nil {
		return Issue{}, err
	}
	if len(found) != 1 {
		return Issue{}, fmt.Errorf("%s has a pending publication with %d marker matches; resolve the ambiguity on the tracker, no POST will be retried", SlotName(slot), len(found))
	}
	if m.Receipt != nil && !m.Receipt.matches(found[0]) {
		return Issue{}, fmt.Errorf("%s stored create receipt does not match the unique marker result; refusing to adopt a substitute", SlotName(slot))
	}
	issue, err := c.Get(ctx, b.Target, found[0].Number)
	if err != nil {
		return Issue{}, err
	}
	if issue.ID != found[0].ID || issue.NodeID != found[0].NodeID || markerCount(issue.Body, marker) != 1 ||
		BodyDigest(found[0].Body) != m.PublishDigest || BodyDigest(issue.Body) != m.PublishDigest {
		return Issue{}, fmt.Errorf("%s readback does not match the pending Issue identity and approved body", SlotName(slot))
	}
	return issue, nil
}
