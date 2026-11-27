package tracker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// ChangeBinding pins the selected tracker, its stable change key, and the
// optional Issue slots (zero for the spec, positive numbers for tasks).
type ChangeBinding struct {
	Target    Target               `yaml:"target"`
	RemoteKey string               `yaml:"remote_key,omitempty"`
	Issues    map[int]IssueBinding `yaml:"issues,omitempty"`
}

func NewChangeBinding(target Target) (*ChangeBinding, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	b := &ChangeBinding{Target: target}
	if target.Kind != "markdown" {
		var key [16]byte
		if _, err := rand.Read(key[:]); err != nil {
			return nil, fmt.Errorf("generate tracker binding key: %w", err)
		}
		b.RemoteKey = hex.EncodeToString(key[:])
	}
	return b, nil
}

func (b ChangeBinding) Validate() error {
	if err := b.Target.Validate(); err != nil {
		return err
	}
	if b.Target.Kind == "markdown" {
		if b.RemoteKey != "" || len(b.Issues) != 0 {
			return fmt.Errorf("markdown tracker cannot have a remote binding key or Issue slots")
		}
		return nil
	}
	decoded, err := hex.DecodeString(b.RemoteKey)
	if err != nil || len(decoded) != 16 || hex.EncodeToString(decoded) != b.RemoteKey {
		return fmt.Errorf("remote tracker requires a valid binding key")
	}
	if b.Target.ProjectID == "" && len(b.Issues) != 0 {
		return fmt.Errorf("Issue slots require a verified project ID")
	}
	ids, numbers, nodes := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	for slot, m := range b.Issues {
		if slot < 0 || m.Path == "" || !validDigest(m.DraftDigest) || !validDigest(m.PublishDigest) {
			return fmt.Errorf("invalid Issue publication record for %s", SlotName(slot))
		}
		if slot != 0 && !b.Issues[0].Published() {
			return fmt.Errorf("task Issue slots require a published spec")
		}
		if m.Receipt != nil && !m.Receipt.validFor(b.Target) {
			return fmt.Errorf("invalid positive create receipt for %s", SlotName(slot))
		}
		if m.ID == "" && m.Number == "" {
			if m.NodeID != "" || m.BodyDigest != "" || m.Baseline != "" {
				return fmt.Errorf("unbound %s has a snapshot", SlotName(slot))
			}
			continue
		}
		if !decimalID(m.ID) || !decimalID(m.Number) || !validDigest(m.BodyDigest) || ids[m.ID] || numbers[m.Number] ||
			(b.Target.Kind == "github" && (!validNodeID(m.NodeID) || nodes[m.NodeID])) ||
			(b.Target.Kind == "gitlab" && m.NodeID != "") {
			return fmt.Errorf("invalid or duplicate Issue identity/snapshot for %s", SlotName(slot))
		}
		if m.Receipt != nil && !m.Receipt.matches(Issue{ID: m.ID, Number: m.Number, NodeID: m.NodeID}) {
			return fmt.Errorf("%s binding disagrees with its positive create receipt", SlotName(slot))
		}
		ids[m.ID], numbers[m.Number], nodes[m.NodeID] = true, true, true
	}
	return nil
}

func validDigest(s string) bool {
	decoded, err := hex.DecodeString(s)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == s
}
