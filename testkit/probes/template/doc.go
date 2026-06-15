// Package templateprobes hosts the openEHR conformance probes for
// the openehr/template/ surface — local ADL 1.4 operational template
// parsing and path resolution (PROBE-022). Probes are plain functions
// returning (Result, error); the template probes are sandbox-only
// (no transport), since the package is a building-block per REQ-013.
package templateprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
