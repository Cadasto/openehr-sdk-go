package basic_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/basic"
)

// redactedMarker is the text a credential prints and logs as.
const redactedMarker = "[redacted]"

// printVerbs are the fmt verbs, with flags and widths, that must all print the
// text of a value's String method.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%t", "%20v", "%-20s", "%+q", "%#x"}

// printProblems prints v through String and GoString, with every verb of
// printVerbs, nested in a struct and a slice, and through the text and JSON
// handlers of log/slog. It returns one line for each output that shows one of
// secrets, for a String text that lacks one of shown, and for a verb whose
// output differs from the String text.
func printProblems(v any, secrets, shown []string) []string {
	var problems []string
	leaks := func(where, out string) {
		for _, s := range secrets {
			if strings.Contains(out, s) {
				problems = append(problems, fmt.Sprintf("%s shows the credential %q: %s", where, s, out))
			}
		}
	}
	stringer, ok := v.(fmt.Stringer)
	if !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Stringer", v))
	}
	if _, ok := v.(fmt.Formatter); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Formatter", v))
	}
	var want string
	if stringer != nil {
		want = stringer.String()
		leaks("String()", want)
		for _, s := range shown {
			if !strings.Contains(want, s) {
				problems = append(problems, fmt.Sprintf("String() = %q, want it to show %q", want, s))
			}
		}
	}
	if g, ok := v.(fmt.GoStringer); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.GoStringer", v))
	} else if got := g.GoString(); got != want {
		problems = append(problems, fmt.Sprintf("GoString() = %q, want %q, the text of String()", got, want))
	}
	for _, verb := range printVerbs {
		out := fmt.Sprintf(verb, v)
		leaks(fmt.Sprintf("fmt.Sprintf(%q)", verb), out)
		if stringer != nil && out != want {
			problems = append(problems, fmt.Sprintf("fmt.Sprintf(%q) = %q, want %q, the text of String()", verb, out, want))
		}
	}
	leaks("a struct field under %+v", fmt.Sprintf("%+v", struct{ V any }{v}))
	leaks("a slice element under %#v", fmt.Sprintf("%#v", []any{v}))
	for name, newHandler := range map[string]func(*bytes.Buffer) slog.Handler{
		"slog JSONHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		"slog TextHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	} {
		var buf bytes.Buffer
		slog.New(newHandler(&buf)).Info("printed", "v", v)
		leaks(name, buf.String())
	}
	return problems
}

// TestREQ060_SourcePrintsNoCredential pins REQ-060: a *basic.Source printed
// with any fmt verb, or logged through log/slog, shows its Basic payload as
// [redacted] and never the payload or the password. A nil *basic.Source
// prints as <nil>.
func TestREQ060_SourcePrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	const password = "pw-s3cr3t"
	src, err := basic.New("alice", password)
	if err != nil {
		t.Fatalf("basic.New: %v", err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte("alice:" + password))
	for _, p := range printProblems(src, []string{payload, password}, []string{redactedMarker, basic.TokenType}) {
		t.Error(p)
	}

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		var nilSrc *basic.Source
		for _, verb := range printVerbs {
			if out := fmt.Sprintf(verb, nilSrc); out != "<nil>" {
				t.Errorf("fmt.Sprintf(%q, a nil *basic.Source) = %q, want <nil>", verb, out)
			}
		}
	})
}
