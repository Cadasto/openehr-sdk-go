package main

import (
	"bufio"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

// The environment the run reads. The CDR variables are the ones the
// repository's Live tests read (testkit/probe); the template directory and
// the output directory are this tool's own.
const (
	envTemplates     = "OPENEHR_XC_TEMPLATES"
	envOut           = "OPENEHR_XC_OUT"
	envAllowMutating = "OPENEHR_LIVE_ALLOW_MUTATING"
)

// modulePath is this repository's module. The output directory may not sit
// inside a checkout of it.
const modulePath = "github.com/cadasto/openehr-sdk-go"

// cdrSpec is one CDR the run can reach: the variable naming its REST base,
// the variable carrying its Basic credential as user:pass, and the example
// levels its template example takes.
//
// The levels are observations, not configuration: EHRbase builds one example
// and takes no detail_level, so it is asked once without one; FerroEHR builds
// the three ITS-REST levels.
type cdrSpec struct {
	name    string
	baseEnv string
	credEnv string
	levels  []definition.ExampleDetailLevel
}

var cdrSpecs = []cdrSpec{
	{
		name:    "ehrbase",
		baseEnv: "OPENEHR_LIVE_EHRBASE",
		credEnv: "OPENEHR_LIVE_EHRBASE_BASIC",
		levels:  []definition.ExampleDetailLevel{""},
	},
	{
		name:    "ferroehr",
		baseEnv: "OPENEHR_LIVE_FERROEHR",
		credEnv: "OPENEHR_LIVE_FERROEHR_BASIC",
		levels: []definition.ExampleDetailLevel{
			definition.ExampleDetailRequired,
			definition.ExampleDetailMedium,
			definition.ExampleDetailComplete,
		},
	},
}

// cdrTarget is a CDR the run talks to. base carries no credential: a base
// URL that does is refused, so the credential lives in user and pass alone.
type cdrTarget struct {
	name   string
	base   string
	user   string
	pass   string
	levels []definition.ExampleDetailLevel
}

// config is one run's settings.
type config struct {
	templates string
	// out is the output directory -out or OPENEHR_XC_OUT names. When
	// neither does, the run makes a new one under outRoot (prepareOut).
	out       string
	outRoot   string
	workers   int
	only      *regexp.Regexp
	sdkCommit string
	// cdrs are the CDRs the run writes to. Empty when no CDR is named or
	// the write opt-in is unset; cdrSkip then says why.
	cdrs    []cdrTarget
	cdrSkip string
}

// skipNotice is printed when the run has nothing to do. The make target exits
// 0 on it.
const skipNotice = "cdrcheck: skipped: set " + envTemplates + " to a directory of OPTs, " +
	"and OPENEHR_LIVE_EHRBASE or OPENEHR_LIVE_FERROEHR (with " + envAllowMutating +
	") to name the CDRs to check them against; not part of make ci"

// loadConfig reads the flags and the environment. It returns skip true when
// no template directory is named, so there is nothing to check. getenv is a
// parameter so tests need not touch the process environment.
//
// No error it returns quotes a value read from the environment: a credential
// variable or a base URL may hold a secret, so a refusal names the variable,
// never its content.
func loadConfig(args []string, getenv func(string) string, usage io.Writer) (cfg config, skip bool, err error) {
	fs := flag.NewFlagSet("cdrcheck", flag.ContinueOnError)
	fs.SetOutput(usage)
	out := fs.String("out", "", "output directory, outside the repository (default $"+envOut+", else a new directory under the system temp dir)")
	workers := fs.Int("workers", 6, "templates handled at once, each CDR call included")
	only := fs.String("only", "", "regular expression a template id must match to be checked (default: every template)")
	sdkCommit := fs.String("sdk-commit", "", "provenance: the SDK commit the run was built from (make passes it)")
	if err := fs.Parse(args); err != nil {
		return config{}, false, err
	}
	if fs.NArg() > 0 {
		return config{}, false, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *workers < 1 {
		return config{}, false, errors.New("-workers must be 1 or more")
	}
	cfg.workers = *workers
	cfg.sdkCommit = *sdkCommit
	if *only != "" {
		re, err := regexp.Compile(*only)
		if err != nil {
			return config{}, false, fmt.Errorf("-only: %w", err)
		}
		cfg.only = re
	}

	cfg.templates = strings.TrimSpace(getenv(envTemplates))
	if cfg.templates == "" {
		return config{}, true, nil
	}
	if fi, err := os.Stat(cfg.templates); err != nil || !fi.IsDir() {
		return config{}, false, fmt.Errorf("%s does not name a directory", envTemplates)
	}

	var named []string
	for _, spec := range cdrSpecs {
		target, ok, err := readTarget(spec, getenv)
		if err != nil {
			return config{}, false, err
		}
		if ok {
			cfg.cdrs = append(cfg.cdrs, target)
			named = append(named, spec.name)
		}
	}
	switch {
	case len(cfg.cdrs) == 0:
		cfg.cdrSkip = "no CDR named: set OPENEHR_LIVE_EHRBASE or OPENEHR_LIVE_FERROEHR"
	case getenv(envAllowMutating) == "":
		// Writing to a deployment is something the operator asks for
		// (conformance.md § REQ-082, Live mode): naming a CDR says where
		// to look, the opt-in says the run may write there.
		cfg.cdrSkip = "set " + envAllowMutating + " to let the run upload templates and create EHRs and compositions on " + strings.Join(named, ", ")
		cfg.cdrs = nil
	}

	given := cmp.Or(*out, getenv(envOut))
	abs, err := filepath.Abs(cmp.Or(given, filepath.Join(os.TempDir(), "openehr-cdrcheck")))
	if err != nil {
		return config{}, false, fmt.Errorf("output directory: %w", err)
	}
	if given != "" {
		cfg.out = abs
	} else {
		cfg.outRoot = abs
	}
	root, inside, err := insideModule(abs)
	if err != nil {
		return config{}, false, fmt.Errorf("output directory %s: cannot tell whether it is inside the repository: %w", abs, err)
	}
	if inside {
		return config{}, false, fmt.Errorf("output directory %s is inside the repository at %s; the run's data stays outside it", abs, root)
	}
	return cfg, false, nil
}

// readTarget reads one CDR's base URL and credential. ok is false when the
// base variable is unset.
func readTarget(spec cdrSpec, getenv func(string) string) (cdrTarget, bool, error) {
	base := strings.TrimSpace(getenv(spec.baseEnv))
	if base == "" {
		return cdrTarget{}, false, nil
	}
	// The URL is never quoted back: it may carry the very credential the
	// check below refuses.
	if channel, found := probe.CredentialInURL(base); found {
		return cdrTarget{}, false, fmt.Errorf("%s carries a credential (%s); pass it in %s instead", spec.baseEnv, channel, spec.credEnv)
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return cdrTarget{}, false, fmt.Errorf("%s is not an absolute http or https URL", spec.baseEnv)
	}
	t := cdrTarget{name: spec.name, base: base, levels: spec.levels}
	if cred := getenv(spec.credEnv); cred != "" {
		user, pass, ok := strings.Cut(cred, ":")
		if !ok || user == "" {
			return cdrTarget{}, false, fmt.Errorf("%s must be user:pass", spec.credEnv)
		}
		t.user, t.pass = user, pass
	}
	return t, true, nil
}

// insideModule reports whether dir is in a checkout of this module, as
// written or with its symbolic links resolved (resolveLinks): dir or one of
// its parents holds a go.mod declaring modulePath. root is that directory.
// A go.mod on the way that cannot be read is an error (declaresModule).
func insideModule(dir string) (root string, inside bool, err error) {
	for _, d := range []string{filepath.Clean(dir), resolveLinks(dir)} {
		if root, inside, err := moduleAt(d); err != nil || inside {
			return root, inside, err
		}
	}
	return "", false, nil
}

// resolveLinks returns dir with the symbolic links of its nearest existing
// ancestor resolved (filepath.EvalSymlinks). The part below that ancestor
// does not exist yet, so it holds no link.
func resolveLinks(dir string) string {
	d, rest := filepath.Clean(dir), ""
	for {
		if r, err := filepath.EvalSymlinks(d); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return filepath.Clean(dir)
		}
		rest = filepath.Join(filepath.Base(d), rest)
		d = parent
	}
}

// moduleAt reports whether dir or one of its parents, as written, holds a
// go.mod declaring modulePath.
func moduleAt(dir string) (root string, inside bool, err error) {
	for d := filepath.Clean(dir); ; {
		ok, err := declaresModule(filepath.Join(d, "go.mod"))
		if err != nil || ok {
			return d, ok, err
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false, nil
		}
		d = parent
	}
}

// declaresModule reports whether the go.mod at path declares modulePath.
// Where there is a go.mod that cannot be read to its module line, it
// returns the error, so that the caller refuses the directory rather than
// take a checkout of this module for another.
func declaresModule(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }() // read-only
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`) == modulePath, nil
		}
	}
	return false, sc.Err()
}
