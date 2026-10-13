package validation

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// Validate validates an in-memory archetypeable RM root against a
// compiled OPT and returns every issue in one pass.
//
// It is the generic form of [ValidateComposition]: the compiled OPT
// drives the lockstep walk and `root` is the value source. `root` must
// be one of the RM LOCATABLE concretes the walker recognises: the
// COMPOSITION content closed set plus the demographic PARTY
// hierarchy (PERSON / ORGANISATION / GROUP / AGENT / ROLE and the
// archetypeable sub-components ADDRESS / CONTACT / PARTY_IDENTITY /
// PARTY_RELATIONSHIP / CAPABILITY) and the EHR IM roots FOLDER /
// EHR_STATUS. A root whose concrete RM type does not match the OPT root
// surfaces as an rm_type_mismatch at "/", not a silent pass.
//
// The result includes the Reference Model floor ([ValidateRM]): after the
// template walk, the floor runs over the whole root, the nodes the template
// does not model included, and its findings follow the template walk's in
// the same Result. A finding the floor makes with the same code at the same
// path as the template walk is dropped, so each defect is reported once.
// No option leaves the floor out; [ValidateRM] runs the floor alone.
//
// Where a template reuses one archetype under several names, an item is
// matched to the template node whose name it carries; an item whose name
// the template does not give goes to such a node that leaves the name
// open, or else to the first of them, whose name check then reports it.
// The item's name decides this match only and appears in no issue path or
// message.
//
// Returns a [Result] whose Issues slice is never nil. A nil root or nil
// compiled template yields a single guard issue (nil_root /
// nil_template). [Result.Coverage] lists what the call did not check.
//
// Validate behaves as a [Validator] built with no options, apart from a nil
// compiled template: Validate reports it, where [Validator.Validate] applies
// the Reference Model rules alone.
func Validate(root any, c *templatecompile.Compiled) Result {
	if root == nil || rmread.IsTypedNilPointer(root) {
		return nilRootResult()
	}
	if c == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "validation called with a nil compiled template argument",
			Severity: Error,
		}})
	}
	var v Validator
	return v.Validate(root, c)
}

// nilRootResult is the guard a template-driven validation returns for a nil
// root.
func nilRootResult() Result {
	return resultFromIssues([]Issue{{
		Path:     "",
		Code:     "nil_root",
		Detail:   "Validate called with a nil RM root argument",
		Severity: Error,
	}})
}

// issueAt is what makes two issues the same finding: the code, at the path.
type issueAt struct {
	code, path string
}

// withoutRepeats returns the template walk's issues, in walk order, followed
// by each of the floor's issues, in walk order, that does not repeat a
// template issue's code at its path. Both walkers write one path per node,
// so an exact match is one defect seen twice; nothing is stripped from a
// path before comparing, which would merge siblings. Neither list is
// deduplicated against itself: the template walk may report one code twice
// at one path for two clauses, and both stay.
func withoutRepeats(template, floor []Issue) []Issue {
	switch {
	case len(floor) == 0:
		return template
	case len(template) == 0:
		return floor
	}
	seen := make(map[issueAt]struct{}, len(template))
	for _, i := range template {
		seen[issueAt{code: i.Code, path: i.Path}] = struct{}{}
	}
	out := slices.Grow(template, len(floor))
	for _, i := range floor {
		if _, repeat := seen[issueAt{code: i.Code, path: i.Path}]; repeat {
			continue
		}
		out = append(out, i)
	}
	return out
}

// ValidateDemographic validates an in-memory demographic PARTY (PERSON,
// ORGANISATION, GROUP, AGENT or ROLE) against a compiled OPT, the
// Reference Model floor included, as [Validate] describes: the floor's
// findings follow the template walk's, less those that repeat one.
// It guards the nil party (yielding nil_party), then delegates to
// [Validate]. The PARTY sub-components (ADDRESS, CONTACT, PARTY_IDENTITY,
// PARTY_RELATIONSHIP, CAPABILITY) are validated in place as the walk
// descends, or as roots in their own right via [Validate].
//
// Because rm.Party is an interface, the guard catches both a bare-nil
// interface and a typed-nil concrete behind it (e.g. a (*rm.Person)(nil)
// argument). Both yield nil_party rather than the generic nil_root.
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result {
	if party == nil || rmread.IsTypedNilPointer(party) {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_party",
			Detail:   "ValidateDemographic called with a nil rm.Party argument",
			Severity: Error,
		}})
	}
	return Validate(party, c)
}

// ValidateFolder validates an in-memory FOLDER (a directory tree root or
// sub-folder) against a compiled OPT, the Reference Model floor included,
// as [Validate] describes: the floor's findings follow the template walk's,
// less those that repeat one. A nil folder yields nil_folder.
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result {
	if folder == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_folder",
			Detail:   "ValidateFolder called with a nil *rm.Folder argument",
			Severity: Error,
		}})
	}
	return Validate(folder, c)
}

// ValidateEHRStatus validates an in-memory EHR_STATUS against a compiled
// OPT, the Reference Model floor included, as [Validate] describes: the
// floor's findings follow the template walk's, less those that repeat one.
// A nil status yields nil_ehr_status.
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result {
	if status == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_ehr_status",
			Detail:   "ValidateEHRStatus called with a nil *rm.EHRStatus argument",
			Severity: Error,
		}})
	}
	return Validate(status, c)
}
