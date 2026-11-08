package probe_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The contribution-audit scenario commits one fixed FOLDER creation under
// a fixed committer, three times, each to an EHR of its own.
const (
	cassetteContributionCommitter = "SDK cassette committer"
	cassetteContributionSystemID  = "org.cadasto.sdk.cassette"
)

// contributionAuditVariant is one commit of the contribution-audit
// scenario, with the audit its recorded request carries and what EHRbase
// 2.37.1 answered.
type contributionAuditVariant struct {
	name      string
	systemID  string                 // "" builds, and records, no system id
	auditType contribution.AuditType // "" keeps the builder's default
	wireType  string                 // the audit _type the recorded request carries
	refusal   string                 // text the recorded 400 message names; "" for a recorded success
}

// contributionAuditVariants are the scenario's commits, in capture order.
var contributionAuditVariants = []contributionAuditVariant{
	{name: "without a system id", wireType: "AUDIT_DETAILS", refusal: "system_id"},
	{name: "with a system id", systemID: cassetteContributionSystemID, wireType: "AUDIT_DETAILS"},
	{
		name: "UPDATE_AUDIT", systemID: cassetteContributionSystemID, auditType: contribution.AuditTypeUpdateAudit,
		wireType: "UPDATE_AUDIT", refusal: "UPDATE_AUDIT",
	},
}

// submission builds v's contribution the way the scenario does: one
// creation of the fixed FOLDER.
func (v contributionAuditVariant) submission() (*contribution.Submission, error) {
	folder := &rm.Folder{
		ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
		Name:            &rm.DVText{Value: "root"},
	}
	b := contribution.NewBuilder().
		WithCommitterName(cassetteContributionCommitter).
		WithChangeType(contribution.ChangeTypeCreation)
	if v.systemID != "" {
		b = b.WithSystemID(v.systemID)
	}
	if v.auditType != "" {
		b = b.WithAuditType(v.auditType)
	}
	return b.Add(contribution.Creation(folder)).Build()
}

// recordedCommits returns the recording's contribution commits in capture
// order. It fails the test unless there is one per variant.
func recordedCommits(t *testing.T, har *probe.HAR) []*probe.HAREntry {
	t.Helper()
	out := recordedEntries(har, "POST", "/contribution")
	if len(out) != len(contributionAuditVariants) {
		t.Fatalf("the recording holds %d contribution commits, want %d", len(out), len(contributionAuditVariants))
	}
	return out
}

// commitAudit is the part of a recorded commit audit the scenario varies.
// A nil SystemID means the audit carries no system_id member.
type commitAudit struct {
	Type     string  `json:"_type"`
	SystemID *string `json:"system_id"`
}

// checkRecordedCommit reports whether the commit recorded in e carried v's
// audit on the batch and on its one version, which inherits it (REQ-130).
// The replayer matches on method and path alone, so only the recording
// shows what each commit sent.
func checkRecordedCommit(v contributionAuditVariant, e *probe.HAREntry) error {
	if e.Request.PostData == nil {
		return fmt.Errorf("commit %s: the recorded request carries no body", v.name)
	}
	var body struct {
		Audit    commitAudit `json:"audit"`
		Versions []struct {
			CommitAudit commitAudit `json:"commit_audit"`
		} `json:"versions"`
	}
	if err := json.Unmarshal([]byte(e.Request.PostData.Text), &body); err != nil {
		return fmt.Errorf("commit %s: recorded request body: %w", v.name, err)
	}
	if len(body.Versions) != 1 {
		return fmt.Errorf("commit %s: the recorded request carries %d versions, want 1", v.name, len(body.Versions))
	}
	for _, a := range []struct {
		where string
		audit commitAudit
	}{{"audit", body.Audit}, {"versions[0].commit_audit", body.Versions[0].CommitAudit}} {
		if a.audit.Type != v.wireType {
			return fmt.Errorf("commit %s: recorded %s _type: got %q, want %q", v.name, a.where, a.audit.Type, v.wireType)
		}
		got := a.audit.SystemID
		if v.systemID == "" && got != nil {
			return fmt.Errorf("commit %s: recorded %s system_id: got %q, want none", v.name, a.where, *got)
		}
		if v.systemID != "" && (got == nil || *got != v.systemID) {
			return fmt.Errorf("commit %s: recorded %s system_id: want %q", v.name, a.where, v.systemID)
		}
	}
	return nil
}

// replayContributionAudit drives the contribution-audit recording through
// c, a client that keeps error bodies, and checks what the SDK makes of
// each recorded answer (REQ-130, REQ-082): the commit without a system id
// is a 400 *transport.WireError whose message names system_id, the commit
// with one succeeds, and the UPDATE_AUDIT commit is a 400 whose message
// names UPDATE_AUDIT. Each recorded request must carry its variant's audit.
func replayContributionAudit(ctx context.Context, c *transport.Client, har probe.HAR) error {
	commits := recordedEntries(&har, "POST", "/contribution")
	for i, v := range contributionAuditVariants {
		if i >= len(commits) {
			return fmt.Errorf("commit %s: the recording holds no commit for it", v.name)
		}
		rec := commits[i]
		if err := checkRecordedCommit(v, rec); err != nil {
			return err
		}
		created, _, err := ehr.Create(ctx, c)
		if err != nil {
			return fmt.Errorf("commit %s: ehr.Create: %w", v.name, err)
		}
		if created == nil || created.EHRID.Value == "" {
			return fmt.Errorf("commit %s: ehr.Create recovered no EHR id", v.name)
		}
		batch, err := v.submission()
		if err != nil {
			return fmt.Errorf("commit %s: build: %w", v.name, err)
		}
		_, _, err = contribution.Commit(ctx, c, ehr.EHRID(created.EHRID.Value), batch)
		if v.refusal == "" {
			if err != nil {
				return fmt.Errorf("commit %s: want success: %w", v.name, err)
			}
			continue
		}
		if err == nil {
			return fmt.Errorf("commit %s: answered, want a 400 refusal", v.name)
		}
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok || we == nil || we.StatusCode != 400 {
			return fmt.Errorf("commit %s: want a 400 wire error: %w", v.name, err)
		}
		if we.OpenEHR == nil || we.OpenEHR.Message != recordedMessage(rec) || !strings.Contains(we.OpenEHR.Message, v.refusal) {
			return fmt.Errorf("commit %s: refusal message: got %+v, want the recorded message naming %q", v.name, we.OpenEHR, v.refusal)
		}
	}
	return nil
}

// TestCassette_ReplaysContributionAudit_REQ130_REQ082 replays EHRbase
// 2.37.1's answers to the builder's commit audits (REQ-130, REQ-082): it
// refuses the default audit without a system id and an UPDATE_AUDIT audit
// with one, and accepts the default audit with a system id. An extra
// create is unmatched.
func TestCassette_ReplaysContributionAudit_REQ130_REQ082(t *testing.T) {
	t.Parallel()
	har := loadRecording(t, "contribution-audit.har")
	c := rawReplayClientFor(t, har)
	if err := replayContributionAudit(t.Context(), c, har); err != nil {
		t.Fatal(err)
	}
	_, _, err := ehr.Create(t.Context(), c)
	wantUnmatched(t, err)
}
