// Command probe-record captures live openEHR REST exchanges into REQ-082
// Cassette recordings (HAR 1.2, ADR 0020). It drives a named scenario through
// a [probe.Recorder] against a live CDR, then gates the result three ways
// before publishing <out>/<scenario>.har:
//
//   - [probe.HAR.Validate] judges the captured document in memory, so a
//     capture carrying a credential is refused before any of it is written —
//     REQ-082 requires that credentials never reach disk, not that they be
//     deleted afterwards.
//   - the capture is replayed through a [probe.Replayer] against a different
//     base URL, driving the same scenario a probe would, so a recording that
//     validates but cannot actually be replayed fails here rather than inside
//     a probe.
//   - only the approved bytes are written, to a sibling temp file that is
//     renamed into place, so a refused capture leaves an existing recording
//     exactly as it was.
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
// (capture-time redaction, verified before publication).
//
// Pass a basic-auth credential in $OPENEHR_LIVE_EHRBASE_BASIC rather than in
// -basic: a flag value is visible in shell history and in process listings.
// A credential in the -base URL is refused outright, because provenance
// records that URL as given.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// replayCheckBase is the base URL the post-capture replay drives against. It
// is deliberately not the capture base: replaying against the URL just
// captured would pass on paths that only match themselves, which is the one
// failure this check exists to catch. The host is in the reserved .invalid
// TLD (RFC 2606) — the replayer never dials, and a name that cannot resolve
// keeps it that way if the transport is ever misassembled.
const replayCheckBase = "https://replay.invalid/openehr/v1"

func main() {
	err := run(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe-record:", err)
		os.Exit(1)
	}
}

// run parses args and captures the selected scenarios. usage is where the
// flag package writes usage and parse errors — a parameter rather than
// os.Stderr outright so TestRun_HelpNeverEchoesTheCredentialEnvVar can read
// what -h actually prints.
func run(args []string, usage io.Writer) error {
	fs := flag.NewFlagSet("probe-record", flag.ContinueOnError)
	fs.SetOutput(usage)
	base := fs.String("base", "", "openEHR REST base URL including the /openehr/v1 suffix (default $OPENEHR_LIVE_EHRBASE)")
	which := fs.String("scenario", "all", "scenario name to capture, or 'all'")
	outDir := fs.String("out", filepath.Join("testkit", "recordings"), "directory to write <scenario>.har into")
	deployment := fs.String("deployment", "", "provenance: the deployment captured, e.g. \"EHRbase 2.35.1\" (required)")
	sdkCommit := fs.String("sdk-commit", buildCommit(), "provenance: sdk_commit (defaults to the built-in VCS revision)")
	cred := fs.String("basic", "", "basic-auth credential as user:pass; prefer $OPENEHR_LIVE_EHRBASE_BASIC, which keeps it out of shell history and process listings")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// The environment fallbacks are read here rather than passed as flag
	// defaults, because flag.PrintDefaults prints a non-empty default: with
	// $OPENEHR_LIVE_EHRBASE_BASIC as the default for -basic, `-h` and every
	// parse error would echo user:pass to stderr. $OPENEHR_LIVE_EHRBASE gets
	// the same treatment — a base URL can carry userinfo.
	if *base == "" {
		*base = os.Getenv("OPENEHR_LIVE_EHRBASE")
	}
	if *cred == "" {
		*cred = os.Getenv("OPENEHR_LIVE_EHRBASE_BASIC")
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
	// provenanceBaseURL trims as well as refusing, and the trimmed form is
	// what both the recording and the dial should use — otherwise a -base with
	// stray whitespace is attested one way and dialed another.
	provBase, err := provenanceBaseURL(*base)
	if err != nil {
		return err
	}
	*base = provBase

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
		BaseURL:    provBase,
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

// provenanceBaseURL returns base as it may be stamped into a recording's
// provenance, refusing one that carries a credential.
//
// This is the one recorded URL redaction does not reach: [probe.Recorder]
// strips userinfo and credential query keys from every entry URL, but
// provenance is copied from -base as given, so a credential there would be
// published (REQ-082: credentials MUST NOT reach disk).
//
// It refuses rather than strips, for two reasons. Stripping would leave the
// operator authenticated on the wire but unable to tell from the recording,
// and the credential has a home already — $OPENEHR_LIVE_EHRBASE_BASIC. The
// refusal names the channel, never the value (REQ-093).
func provenanceBaseURL(base string) (string, error) {
	base = strings.TrimSpace(base)
	if _, err := url.Parse(base); err != nil {
		// The parse error is dropped rather than wrapped: its message quotes
		// the URL it failed on, which is exactly the text a refusal must not
		// echo when that URL is the one carrying a credential (REQ-093).
		return "", errors.New("-base does not parse as a URL")
	}
	if channel, found := probe.CredentialInURL(base); found {
		return "", fmt.Errorf("-base carries a credential (%s); pass it in $OPENEHR_LIVE_EHRBASE_BASIC instead", channel)
	}
	return base, nil
}

// captureScenario drives sc through a [probe.Recorder] wrapping next against
// base, and publishes outDir/<sc.name>.har only if the capture both validates
// and replays. next is the real transport for a live capture; a test passes
// the sandbox.
//
// The order is the contract: validate the document in memory, replay it, then
// write. Writing first and checking afterwards would put a leaked credential
// on disk and could replace a good recording with a refused one — see
// TestCaptureScenario_RefusedCaptureLeavesTheCorpusUntouched.
func captureScenario(ctx context.Context, sc scenario, base string, next http.RoundTripper, tok auth.TokenSource, prov probe.HARProvenance, outDir string) (string, error) {
	rec := probe.NewRecorder(next, prov)
	c, err := probe.NewClient(base, &http.Client{Transport: rec, Timeout: 30 * time.Second}, tok)
	if err != nil {
		return "", err
	}
	if err := sc.capture(ctx, c); err != nil {
		return "", err
	}

	har := rec.HAR()
	if err := har.Validate(); err != nil {
		return "", fmt.Errorf("capture did not validate: %w", err)
	}
	if err := replayCheck(ctx, sc, har); err != nil {
		return "", fmt.Errorf("capture validated but did not replay: %w", err)
	}

	data, err := json.MarshalIndent(har, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal HAR: %w", err)
	}
	return publish(outDir, sc.name+".har", append(data, '\n'))
}

// replayCheck drives sc a second time against a [probe.Replayer] over har, at
// a base URL other than the one captured, and reports what a probe replaying
// this recording would hit.
//
// [probe.HAR.Validate] judges shape and redaction; it cannot tell whether the
// recorded exchanges answer the requests the scenario makes. A truncated
// capture, or one whose recorded paths only match the deployment they came
// from, passes validation and then fails inside a probe with
// [probe.ErrUnmatchedRecording] — see
// TestReplayCheck_RefusesATruncatedRecording. Replay reaches no network: the
// replayer serves recorded exchanges and refuses everything else (REQ-082).
func replayCheck(ctx context.Context, sc scenario, har probe.HAR) error {
	c, err := probe.NewClient(replayCheckBase, probe.NewReplayer(har).HTTPClient(), nil)
	if err != nil {
		return err
	}
	return sc.capture(ctx, c)
}

// publish writes data to dir/name atomically: a sibling temp file, then a
// rename. The temp file never holds unapproved bytes — the caller has already
// validated them — and the rename means a recording is either the previous
// one or the new one, never a half-written file. A failure removes the temp
// and leaves any existing recording alone.
func publish(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create out dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, name+".tmp*")
	if err != nil {
		return "", fmt.Errorf("create temp recording: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write recording: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("write recording: %w", err)
	}
	// CreateTemp makes the file 0600; a corpus recording is committed and
	// world-readable like every other file in the tree.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", fmt.Errorf("set recording mode: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("publish recording: %w", err)
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
