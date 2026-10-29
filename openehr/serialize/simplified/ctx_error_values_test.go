package simplified_test

// REQ-052 / REQ-053 — the FLAT ctx/ refusals a caller reads name keys and
// RM attributes, never the document's values: a composer name or a setting
// rubric is patient-adjacent data, and an error ends up in logs.
import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// TestREQ053CtxConflictNamesKeysNotValues — REQ-052, REQ-053. Two spellings of
// one ctx/ value that disagree are refused naming the ctx/ key and both FLAT
// keys, and carrying neither value.
func TestREQ053CtxConflictNamesKeysNotValues(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	const first, second = "Dr Alice Secret-0451", "Dr Bob Secret-0452"
	for _, tc := range []struct {
		name, ctxKey, realPath string
	}{
		{"composer name", "ctx/composer_name", "/composer|name"},
		{"language", "ctx/language", "/language|code"},
		{"start time", "ctx/time", "/context/start_time"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := reflatten(t, comp, wt, func(root string, m map[string]any) {
				m[tc.ctxKey] = first
				m[root+tc.realPath] = second
			})
			_, err := simplified.UnmarshalFlat(in, wt)
			if !errors.Is(err, simplified.ErrUnknownPath) {
				t.Fatalf("UnmarshalFlat err = %v, want ErrUnknownPath", err)
			}
			msg := err.Error()
			for _, key := range []string{tc.ctxKey, wt.Tree.ID + tc.realPath} {
				if !strings.Contains(msg, key) {
					t.Errorf("err = %q, want it to name %q", msg, key)
				}
			}
			for _, value := range []string{first, second} {
				if strings.Contains(msg, value) {
					t.Errorf("err = %q carries the value %q", msg, value)
				}
			}
		})
	}
}

// TestREQ053SettingEncodeRefusalsCarryNoValue — REQ-052, REQ-053. Each
// EVENT_CONTEXT.setting the ctx/setting pair cannot carry is refused naming
// ctx/setting and EVENT_CONTEXT.setting, and carrying none of the setting's
// value, code string or terminology id.
func TestREQ053SettingEncodeRefusalsCarryNoValue(t *testing.T) {
	const (
		value = "secret-rubric-0451"
		code  = "secret-code-0451"
		term  = "SECRET-TERM-0451"
	)
	setting := func() rm.DVCodedText {
		return rm.DVCodedText{
			Value:        value,
			DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "openehr"}},
		}
	}
	str := func(s string) *string { return &s }
	for _, tc := range []struct {
		name string
		mut  func(*rm.DVCodedText)
	}{
		{"value without code", func(s *rm.DVCodedText) { s.DefiningCode.CodeString = "" }},
		{"another terminology", func(s *rm.DVCodedText) { s.DefiningCode.TerminologyID.Value = term }},
		{"empty terminology", func(s *rm.DVCodedText) { s.DefiningCode.TerminologyID.Value = "" }},
		{"extras", func(s *rm.DVCodedText) { s.DefiningCode.PreferredTerm = str(value) }},
		{"code without value", func(s *rm.DVCodedText) { s.Value = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, wt := genComposition(t, minimalObsOPT)
			s := setting()
			tc.mut(&s)
			comp.Context.Setting = s
			_, err := simplified.MarshalFlat(comp, wt)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Fatalf("MarshalFlat err = %v, want ErrUnsupportedDatatype", err)
			}
			msg := err.Error()
			for _, name := range []string{"ctx/setting", "EVENT_CONTEXT.setting"} {
				if !strings.Contains(msg, name) {
					t.Errorf("err = %q, want it to name %q", msg, name)
				}
			}
			for _, v := range []string{value, code, term} {
				if strings.Contains(msg, v) {
					t.Errorf("err = %q carries the setting's %q", msg, v)
				}
			}
		})
	}
}
