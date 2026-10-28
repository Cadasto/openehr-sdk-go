package compositionprobes

// Probe023WithRoundTrip runs PROBE-023 with roundTrip as its second leg,
// the decode and re-marshal step.
var Probe023WithRoundTrip = probe023

// CanjsonRoundTrip is the second leg Probe023BuilderRoundTrip uses.
var CanjsonRoundTrip = canjsonRoundTrip
