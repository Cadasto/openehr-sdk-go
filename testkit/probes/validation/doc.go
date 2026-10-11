// Package validationprobes hosts the openEHR conformance probes
// for openehr/validation/: template-driven RM validation against a
// compiled OPT. Probes are plain functions returning (Result, error).
// They need no client or server, because the validator is a local
// building block.
//
// PROBE-025 covers the canonical fixture path: a complete
// composition validates clean, while structural / primitive
// mutations produce a stable multiset of issue codes that
// downstream SDKs must reproduce. PROBE-026 sharpens the negative
// surface with missing-node and cardinality cases.
//
// PROBE-074 extends the conformance surface beyond COMPOSITION:
// the same compiled-OPT walker validates the demographic
// PARTY hierarchy (PERSON / ORGANISATION / GROUP / AGENT / ROLE +
// sub-components) and the EHR-IM roots FOLDER / EHR_STATUS, asserting
// the issue-code multiset per (OPT, root) shape via [validation.Validate].
//
// Template-driven validation includes the RM floor, so each multiset holds
// the floor's findings too, a defect both layers find once. The fixtures
// are valid against the RM where a case means to test the template alone.
package validationprobes

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the shared probe outcome, an alias of [probe.Result].
type Result = probe.Result
