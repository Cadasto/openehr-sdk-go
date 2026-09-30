package probeclass

import "github.com/cadasto/openehr-sdk-go/testkit/probe"

// Result is the one runner member a probe package may name.
type Result = probe.Result

// runner names another runner member, which the class check refuses.
var runner = probe.Run
