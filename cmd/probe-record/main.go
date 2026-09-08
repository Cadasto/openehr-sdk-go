// Command probe-record captures live openEHR REST exchanges into REQ-082
// Cassette recordings (HAR 1.2, ADR 0020). It drives a named scenario through
// a [probe.Recorder] against a live CDR, writes <out>/<scenario>.har, and
// re-reads it through [probe.ValidateHAR] so a capture that cannot be replayed
// or that leaked a credential fails here rather than later inside a probe.
//
// Capturing needs a reachable deployment: a recording is a conformance witness
// only because it came from a real CDR (testkit/recordings/README.md). Replay,
// by contrast, is offline — that is what CI runs.
//
// Usage:
//
//	go run ./cmd/probe-record \
//	  -base http://localhost:8080/ehrbase/rest/openehr/v1 \
//	  -deployment "EHRbase 2.35.1" \
//	  -scenario ehr-lifecycle
//
// A mutating scenario writes to the target CDR — point it at a disposable
// deployment. Credentials stay on the wire but never reach the recording
// (capture-time redaction, verified by ValidateHAR).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

func main() {
	err := run(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe-record:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("probe-record", flag.ContinueOnError)
	base := fs.String("base", os.Getenv("OPENEHR_LIVE_EHRBASE"), "openEHR REST base URL including the /openehr/v1 suffix (default $OPENEHR_LIVE_EHRBASE)")
	which := fs.String("scenario", "all", "scenario name to capture, or 'all'")
	outDir := fs.String("out", filepath.Join("testkit", "recordings"), "directory to write <scenario>.har into")
	deployment := fs.String("deployment", "", "provenance: the deployment captured, e.g. \"EHRbase 2.35.1\" (required)")
	sdkCommit := fs.String("sdk-commit", buildCommit(), "provenance: sdk_commit (defaults to the built-in VCS revision)")
	cred := fs.String("basic", os.Getenv("OPENEHR_LIVE_EHRBASE_BASIC"), "optional basic-auth credential as user:pass (default $OPENEHR_LIVE_EHRBASE_BASIC)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *base == "" {
		return errors.New("-base (or $OPENEHR_LIVE_EHRBASE) is required")
	}
	if *deployment == "" {
		return errors.New("-deployment is required — it is the recording's provenance")
	}
	if *sdkCommit == "" {
		return errors.New("-sdk-commit is required — build stamping is unavailable, so pass it")
	}

	tok, err := tokenSource(*cred)
	if err != nil {
		return err
	}
	selected, err := selectScenarios(*which)
	if err != nil {
		return err
	}

	prov := probe.HARProvenance{
		Deployment: *deployment,
		BaseURL:    *base,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		SDKCommit:  *sdkCommit,
	}

	ctx := context.Background()
	for _, sc := range selected {
		path, err := captureScenario(ctx, sc, *base, http.DefaultTransport, tok, prov, *outDir)
		if err != nil {
			return fmt.Errorf("capture %q: %w", sc.name, err)
		}
		fmt.Printf("captured %s — %s\n", path, sc.desc)
	}
	return nil
}

// captureScenario drives sc through a [probe.Recorder] wrapping next against
// base, writes the HAR to outDir/<sc.name>.har, and re-reads it through
// [probe.ValidateHAR] so an unreplayable or unredacted capture fails here.
// next is the real transport for a live capture; a test passes the sandbox.
func captureScenario(ctx context.Context, sc scenario, base string, next http.RoundTripper, tok auth.TokenSource, prov probe.HARProvenance, outDir string) (string, error) {
	rec := probe.NewRecorder(next, prov)
	c, err := probe.NewClient(base, &http.Client{Transport: rec, Timeout: 30 * time.Second}, tok)
	if err != nil {
		return "", err
	}
	if err := sc.capture(ctx, c); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(rec.HAR(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal HAR: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create out dir: %w", err)
	}
	path := filepath.Join(outDir, sc.name+".har")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write recording: %w", err)
	}
	if _, err := probe.ValidateHAR(path); err != nil {
		return "", fmt.Errorf("captured %s but it did not validate: %w", path, err)
	}
	return path, nil
}

// tokenSource builds a basic-auth token source from a user:pass credential, or
// returns nil for an anonymous target (an open dev CDR needs none).
func tokenSource(cred string) (auth.TokenSource, error) {
	if cred == "" {
		return nil, nil
	}
	user, pass, ok := strings.Cut(cred, ":")
	if !ok {
		return nil, errors.New("-basic must be user:pass")
	}
	return basic.New(user, pass)
}

func selectScenarios(which string) ([]scenario, error) {
	if which == "all" {
		out := make([]scenario, 0, len(scenarios))
		for _, sc := range scenarios {
			out = append(out, sc)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
		return out, nil
	}
	sc, ok := scenarios[which]
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q (known: %s)", which, strings.Join(scenarioNames(), ", "))
	}
	return []scenario{sc}, nil
}

// buildCommit returns the VCS revision the binary was built from, or "" when
// the toolchain did not stamp one (as `go run` on a dirty tree may not).
func buildCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
