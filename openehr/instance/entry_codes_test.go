package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_EntryCodesWhenOPTNamesNoCodeOnEveryEntry is the REQ-107 check
// that every ENTRY concrete whose OPT names language and encoding without
// a code, as a bare CODE_PHRASE or as a C_CODE_PHRASE with an empty code
// list, takes Options.Language in ISO_639-1 and UTF-8 in
// IANA_character-sets, under both policies, both value fills and both
// compile modes, and passes the RM floor.
func TestREQ107_EntryCodesWhenOPTNamesNoCodeOnEveryEntry(t *testing.T) {
	shapes := []struct {
		name               string
		language, encoding string
	}{
		{name: "bare CODE_PHRASE", language: optNode("CODE_PHRASE", ""), encoding: optNode("CODE_PHRASE", "")},
		{name: "empty-list C_CODE_PHRASE", language: optCodePhrase("ISO_639-1"), encoding: optCodePhrase("IANA_character-sets")},
	}
	wantLanguage := rm.CodePhrase{CodeString: namedLanguage, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
	wantEncoding := rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
	for _, class := range entryConcretes {
		for _, shape := range shapes {
			opt := optTemplate(class, optSingle("language", shape.language), optSingle("encoding", shape.encoding))
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, opt, implicit)
				for _, opts := range defaultsOptions() {
					opts.Language = namedLanguage
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", class, shape.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						language, encoding, ok := entryLanguageEncoding(out)
						if !ok {
							t.Fatalf("generated root is %T, want %s", out, class)
						}
						checkCode(t, class+".language", language, wantLanguage)
						checkCode(t, class+".encoding", encoding, wantEncoding)
						noFloorErrors(t, out)
					})
				}
			}
		}
	}
}
