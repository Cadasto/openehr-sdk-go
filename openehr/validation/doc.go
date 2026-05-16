// Package validation provides validation interfaces and
// implementations for openEHR artifacts:
//
//   - Composition validation against an OPT.
//   - Demographic resource structural validation.
//   - AQL syntax and path resolution.
//
// Independently usable without HTTP or auth — suitable for CI
// validators, webhook handlers, and pre-commit hooks in
// clinical-modeling repos.
//
// validation must not take on the codec's dependencies — clean
// separation from openehr/serialize is enforced.
package validation
