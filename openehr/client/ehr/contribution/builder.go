package contribution

import (
	"errors"
	"fmt"
	"slices"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// ChangeType is an openEHR *audit change type* terminology code, carried as
// the `commit_audit.change_type` of a contributed version. The value set is
// closed; its normative home is docs/specifications/wire.md § REQ-130, where
// `523` is the deletion code — `253` is *unknown*, not *deleted*. A code
// outside this set is reachable only by supplying a whole [UpdateAudit] via
// [Builder.WithAudit].
type ChangeType string

const (
	// ChangeTypeCreation is code 249 ("creation") — a first version.
	ChangeTypeCreation ChangeType = "249"
	// ChangeTypeAmendment is code 250 ("amendment").
	ChangeTypeAmendment ChangeType = "250"
	// ChangeTypeModification is code 251 ("modification").
	ChangeTypeModification ChangeType = "251"
	// ChangeTypeDeleted is code 523 ("deleted") — note 523, not 253.
	ChangeTypeDeleted ChangeType = "523"
)

// changeTypeTerms maps each code to its openEHR terminology rubric. Both
// halves reach the wire: a CDR reads the code, a human reads the value.
var changeTypeTerms = map[ChangeType]string{
	ChangeTypeCreation:     "creation",
	ChangeTypeAmendment:    "amendment",
	ChangeTypeModification: "modification",
	ChangeTypeDeleted:      "deleted",
}

// IsValid reports whether c is one of the openEHR audit-change-type codes
// this SDK authors.
func (c ChangeType) IsValid() bool {
	_, ok := changeTypeTerms[c]
	return ok
}

// CodedText renders c as the DV_CODED_TEXT the write-side audit carries
// (nested `defining_code`, terminology `openehr`) — the shape ITS-REST
// PR 131 requires in place of the withdrawn flat TERMINOLOGY_CODE. An
// unrecognised code yields the zero value; check [ChangeType.IsValid]
// first when the code came from outside the SDK.
func (c ChangeType) CodedText() rm.DVCodedText {
	term, ok := changeTypeTerms[c]
	if !ok {
		return rm.DVCodedText{}
	}
	return codedText(term, string(c))
}

// lifecycleTerms maps each openEHR *version lifecycle state* code to its
// rubric. The code set itself lives on [openehrclient.LifecycleState],
// which the `openehr-version` header path shares.
var lifecycleTerms = map[openehrclient.LifecycleState]string{
	openehrclient.LifecycleStateComplete:   "complete",
	openehrclient.LifecycleStateIncomplete: "incomplete",
	openehrclient.LifecycleStateDeleted:    "deleted",
}

// codedText builds an openEHR-terminology DV_CODED_TEXT.
func codedText(value, code string) rm.DVCodedText {
	return rm.DVCodedText{
		DVText: rm.DVText{Value: value},
		DefiningCode: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "openehr"},
			CodeString:    code,
		},
	}
}

// Versionable is the closed set of RM types a CONTRIBUTION may commit —
// the four the ITS-REST `Contribution_create` schema admits. It constrains
// [Creation], [Amendment], [Modification], and [Deletion] so the type-set
// is enforced by the compiler at the call site, with no reflection
// (REQ-024).
type Versionable interface {
	rm.Composition | rm.EHRStatus | rm.Folder | rm.EHRAccess
}

// Change is one accumulated version-to-commit, produced by [Creation],
// [Amendment], [Modification], or [Deletion] and handed to
// [Builder.Add]. It is inert: the write-side version is assembled at
// [Builder.Build], once the batch audit it inherits from is known. A
// Change that could not be formed carries its error to Build rather than
// panicking at the call that made it (REQ-025) — a fluent chain has
// nowhere to return one.
type Change struct {
	build func(batch UpdateAudit) (CommitVersion, error)
	err   error
}

// VersionOption refines one [Change]. Options that name a field of the
// commit audit override what the version would otherwise inherit from the
// batch audit.
type VersionOption func(*versionConfig)

type versionConfig struct {
	lifecycle   openehrclient.LifecycleState
	uid         string
	committer   rm.PartyProxy
	description rm.DVTextLike
	systemID    string
	hasSystemID bool
}

// WithLifecycleState sets this version's `lifecycle_state`, defaulting to
// `complete` (532). It is carried in the version body, not the
// `openehr-version` header: the header is per-request and cannot express a
// distinct state for each version of a multi-version contribution
// (REQ-130, REQ-059).
func WithLifecycleState(s openehrclient.LifecycleState) VersionOption {
	return func(c *versionConfig) { c.lifecycle = s }
}

// WithVersionUID sets this version's `uid`. The server assigns one at
// commit, so an unset uid is omitted from the body rather than sent empty;
// supply one only when the target deployment expects it.
func WithVersionUID(uid string) VersionOption {
	return func(c *versionConfig) { c.uid = uid }
}

// WithVersionCommitter overrides the batch committer for this version.
func WithVersionCommitter(p rm.PartyProxy) VersionOption {
	return func(c *versionConfig) { c.committer = p }
}

// WithVersionDescription overrides the batch audit description for this
// version.
func WithVersionDescription(text string) VersionOption {
	return func(c *versionConfig) { c.description = &rm.DVText{Value: text} }
}

// WithVersionSystemID overrides the batch system id for this version.
func WithVersionSystemID(id string) VersionOption {
	return func(c *versionConfig) {
		c.systemID = id
		c.hasSystemID = true
	}
}

// Creation accumulates a first version of data — change type `creation`
// (249). A creation carries no `preceding_version_uid`: the version it
// would follow does not exist yet.
func Creation[T Versionable](data *T, opts ...VersionOption) Change {
	return newChange(ChangeTypeCreation, "", data, opts...)
}

// Amendment accumulates an amendment of the version at precedingUID —
// change type `amendment` (250).
func Amendment[T Versionable](precedingUID string, data *T, opts ...VersionOption) Change {
	return newChange(ChangeTypeAmendment, precedingUID, data, opts...)
}

// Modification accumulates a modification of the version at precedingUID —
// change type `modification` (251).
func Modification[T Versionable](precedingUID string, data *T, opts ...VersionOption) Change {
	return newChange(ChangeTypeModification, precedingUID, data, opts...)
}

// Deletion accumulates a logical deletion of the version at precedingUID —
// change type `deleted` (523). The version's `lifecycle_state` is NOT
// derived from the change type: it defaults to `complete` like any other
// version, since a deletion of complete content is exactly what most of the
// vendored corpus records. Pass [WithLifecycleState] to say otherwise.
func Deletion[T Versionable](precedingUID string, data *T, opts ...VersionOption) Change {
	return newChange(ChangeTypeDeleted, precedingUID, data, opts...)
}

// newChange captures the caller's payload and options in a closure that
// assembles the write-side version once Build resolves the batch audit.
// T is instantiated at the call site, so the type-set is a compile-time
// fact and the Builder itself stays non-generic — Go methods cannot carry
// type parameters.
func newChange[T Versionable](ct ChangeType, precedingUID string, data *T, opts ...VersionOption) Change {
	if data == nil {
		return Change{err: fmt.Errorf("contribution: %s change has nil data", changeTypeTerms[ct])}
	}
	if ct != ChangeTypeCreation && precedingUID == "" {
		return Change{err: fmt.Errorf("contribution: %s change needs a preceding version uid", changeTypeTerms[ct])}
	}
	cfg := versionConfig{lifecycle: openehrclient.LifecycleStateComplete}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if !cfg.lifecycle.IsValid() {
		return Change{err: fmt.Errorf("contribution: %q is not an openEHR version-lifecycle-state code", string(cfg.lifecycle))}
	}
	return Change{build: func(batch UpdateAudit) (CommitVersion, error) {
		ov := &rm.OriginalVersion[T]{
			Data:           data,
			LifecycleState: codedText(lifecycleTerms[cfg.lifecycle], string(cfg.lifecycle)),
		}
		if cfg.uid != "" {
			ov.UID = rm.ObjectVersionID{Value: cfg.uid}
		}
		if precedingUID != "" {
			ov.PrecedingVersionUID = &rm.ObjectVersionID{Value: precedingUID}
		}
		return &OriginalVersion[T]{Version: ov, CommitAudit: cfg.audit(batch, ct)}, nil
	}}
}

// audit resolves this version's commit_audit: the operation's change type,
// over the batch audit's committer / system id / description / _type,
// with any per-version override applied.
func (c versionConfig) audit(batch UpdateAudit, ct ChangeType) UpdateAudit {
	a := UpdateAudit{
		ChangeType:  ct.CodedText(),
		Committer:   batch.Committer,
		Description: batch.Description,
		SystemID:    batch.SystemID,
		Type:        batch.Type,
	}
	if c.committer != nil {
		a.Committer = c.committer
	}
	if c.description != nil {
		a.Description = c.description
	}
	if c.hasSystemID {
		a.SystemID = c.systemID
	}
	return a
}

// Builder assembles a [Submission] — the ITS-REST `Contribution_create`
// body — from caller payloads, without hand-wiring version wrappers,
// change-type codes, or write-side audit fields (REQ-130). Construct with
// [NewBuilder], accumulate with [Builder.Add], finalise with
// [Builder.Build].
//
// Every method returns the same Builder so calls chain; errors are
// accumulated and surfaced from Build, mirroring the composition builder
// (REQ-101). A Builder is not safe for concurrent use; build one per
// goroutine.
type Builder struct {
	audit   UpdateAudit
	changes []Change
	errs    []error
}

// NewBuilder returns an empty Builder. The batch audit's `change_type` and
// `committer` are required by the pin and have no default — set them via
// [Builder.WithChangeType] and [Builder.WithCommitter] (or supply a whole
// audit with [Builder.WithAudit]) before calling Build.
func NewBuilder() *Builder { return &Builder{} }

// WithAudit replaces the batch commit audit wholesale. Later WithCommitter
// / WithSystemID / WithDescription / WithChangeType calls refine it.
func (b *Builder) WithAudit(a UpdateAudit) *Builder {
	if b == nil {
		return b
	}
	b.audit = a
	return b
}

// WithChangeType sets the batch audit's change type. It describes the
// contribution as a whole and is never derived from the accumulated
// versions — the vendored corpus records batches whose audit change type
// matches none of their versions (REQ-130).
func (b *Builder) WithChangeType(ct ChangeType) *Builder {
	if b == nil {
		return b
	}
	if !ct.IsValid() {
		b.errs = append(b.errs, fmt.Errorf("contribution.Builder: %q is not an openEHR audit-change-type code", string(ct)))
		return b
	}
	b.audit.ChangeType = ct.CodedText()
	return b
}

// WithCommitter sets the batch committer, inherited by every version that
// does not override it. The concrete PartyProxy must be a pointer so its
// `_type` discriminator is emitted.
func (b *Builder) WithCommitter(p rm.PartyProxy) *Builder {
	if b == nil {
		return b
	}
	b.audit.Committer = p
	return b
}

// WithCommitterName sets the batch committer to a PARTY_IDENTIFIED bearing
// only name — the common case. Use [Builder.WithCommitter] when the
// committer needs an external reference or identifiers.
func (b *Builder) WithCommitterName(name string) *Builder {
	if b == nil {
		return b
	}
	return b.WithCommitter(&rm.PartyIdentified{Name: &name})
}

// WithDescription sets the batch audit's reason-for-committal text.
func (b *Builder) WithDescription(text string) *Builder {
	if b == nil {
		return b
	}
	b.audit.Description = &rm.DVText{Value: text}
	return b
}

// WithSystemID sets the batch audit's logical system id. It is optional on
// the write path — the server sets its own when omitted.
func (b *Builder) WithSystemID(id string) *Builder {
	if b == nil {
		return b
	}
	b.audit.SystemID = id
	return b
}

// WithAuditType selects the `_type` emitted on the batch audit and on every
// version audit that inherits it — see [AuditType].
func (b *Builder) WithAuditType(t AuditType) *Builder {
	if b == nil {
		return b
	}
	b.audit.Type = t
	return b
}

// Add accumulates changes in order. A nil-data or otherwise malformed
// Change is kept and reported by Build, so a chain never loses an error.
func (b *Builder) Add(changes ...Change) *Builder {
	if b == nil {
		return b
	}
	b.changes = append(b.changes, changes...)
	return b
}

// Build assembles the accumulated changes into a [Submission] that passes
// [Submission.Validate], or returns every accumulated error joined and no
// submission. It is idempotent: the returned submission is freshly
// allocated, so a second Build — or any later mutation of the Builder —
// leaves an earlier result untouched.
func (b *Builder) Build() (*Submission, error) {
	if b == nil {
		return nil, errors.New("contribution: nil Builder")
	}
	// Clone rather than append onto b.errs: appending into its spare
	// capacity would write through the shared backing array, so a second
	// Build would be scribbling over the first one's working slice.
	errs := slices.Clone(b.errs)
	if len(b.changes) == 0 {
		errs = append(errs, errors.New("contribution.Builder: no versions added (Contribution_create requires at least one)"))
	}
	// change_type and committer are both required on the pin's write-side
	// audit DTO. Validate covers the committer for every submission; the
	// change type is the builder's own gate, since an audit that reached
	// Validate with an empty code would ship a body the pin rejects.
	if b.audit.ChangeType.DefiningCode.CodeString == "" {
		errs = append(errs, errors.New("contribution.Builder: batch audit change_type is required — set it with WithChangeType"))
	}
	versions := make([]CommitVersion, 0, len(b.changes))
	for i, ch := range b.changes {
		switch {
		case ch.err != nil:
			errs = append(errs, fmt.Errorf("change %d: %w", i, ch.err))
		case ch.build == nil:
			errs = append(errs, fmt.Errorf("change %d: zero Change — construct it with Creation/Amendment/Modification/Deletion", i))
		default:
			v, err := ch.build(b.audit)
			if err != nil {
				errs = append(errs, fmt.Errorf("change %d: %w", i, err))
				continue
			}
			versions = append(versions, v)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	sub := &Submission{Audit: b.audit, Versions: versions}
	if err := sub.Validate(); err != nil {
		return nil, fmt.Errorf("contribution.Builder: %w", err)
	}
	return sub, nil
}
