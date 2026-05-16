// Package definitionprobes hosts the cross-SDK conformance probes
// for the openEHR REST Definition API (PROBE-067 today; more follow
// alongside ADL 2 and stored-AQL surfaces).
//
// Probes are plain functions returning (Result, error) and run
// against any compliant transport.Client backend — sandbox, cassette,
// or live deployment.
package definitionprobes

// Result captures the outcome of a probe invocation. Status is
// "pass" / "fail" / "skip"; Detail is human-readable supplementary
// text for failures or skips.
type Result struct {
	Probe  string
	Status string
	Detail string
}
