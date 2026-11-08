package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The contribution-audit scenario commits one fixed FOLDER creation under
// a fixed committer. Fixed values keep the recorded bodies the same on
// every capture; only the EHR ids in the paths change.
const (
	cassetteContributionCommitter = "SDK cassette committer"
	cassetteContributionSystemID  = "org.cadasto.sdk.cassette"
	cassetteFolderArchetypeID     = "openEHR-EHR-FOLDER.generic.v1"
	cassetteFolderName            = "root"
)

// contributionAuditVariant is one commit of the contribution-audit
// scenario: the builder's default audit, with or without a system id, or
// the UPDATE_AUDIT type with one.
type contributionAuditVariant struct {
	name      string
	systemID  string
	auditType contribution.AuditType
	// mustCommit marks the variant every deployment accepts. The others
	// may be accepted or refused with a 4xx, so the scenario records any
	// deployment.
	mustCommit bool
}

// contributionAuditVariants are the scenario's three commits, in capture
// order. The UPDATE_AUDIT commit differs from the one before it in the
// audit type alone.
var contributionAuditVariants = []contributionAuditVariant{
	{name: "without a system id"},
	{name: "with a system id", systemID: cassetteContributionSystemID, mustCommit: true},
	{name: "UPDATE_AUDIT", systemID: cassetteContributionSystemID, auditType: contribution.AuditTypeUpdateAudit},
}

// submission builds v's contribution: one creation of the fixed FOLDER.
func (v contributionAuditVariant) submission() (*contribution.Submission, error) {
	folder := &rm.Folder{
		ArchetypeNodeID: cassetteFolderArchetypeID,
		Name:            &rm.DVText{Value: cassetteFolderName},
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

// captureContributionAudit records what a deployment makes of the commit
// audits the builder emits: each variant commits the same FOLDER to an EHR
// of its own, so every commit has its own path. The commit with a system
// id must succeed. The others may succeed or be refused with a 4xx; any
// other answer fails the capture.
func captureContributionAudit(ctx context.Context, c *transport.Client) error {
	for _, v := range contributionAuditVariants {
		rec, _, err := ehr.Create(ctx, c)
		if err != nil {
			return fmt.Errorf("commit %s: create EHR: %w", v.name, err)
		}
		if rec == nil || rec.EHRID.Value == "" {
			return fmt.Errorf("commit %s: create EHR returned no ehr_id", v.name)
		}
		batch, err := v.submission()
		if err != nil {
			return fmt.Errorf("commit %s: build: %w", v.name, err)
		}
		_, _, err = contribution.Commit(ctx, c, ehr.EHRID(rec.EHRID.Value), batch)
		if err == nil {
			continue
		}
		if v.mustCommit {
			return fmt.Errorf("commit %s: want success: %w", v.name, err)
		}
		if we, ok := errors.AsType[*transport.WireError](err); !ok || we == nil || we.StatusCode/100 != 4 {
			return fmt.Errorf("commit %s: want success or a 4xx refusal: %w", v.name, err)
		}
	}
	return nil
}
