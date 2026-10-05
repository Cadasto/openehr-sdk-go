package instance

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// typeArgumentGoTypes maps each RM type builtTypeArgument names to the Go
// type the generated RM package uses for it.
var typeArgumentGoTypes = map[string]reflect.Type{
	"ITEM_STRUCTURE": reflect.TypeFor[rm.ItemStructure](),
}

// parameterTyped returns the attributes of class whose BMM type is one of
// the class's formal generic parameters, and whether each is required.
func parameterTyped(t *testing.T, class string) map[string]bool {
	t.Helper()
	names, ok := rminfo.Default.(interface{ AttributeNames(string) []string })
	if !ok {
		t.Fatal("rminfo.Default lists no attribute names")
	}
	required := rminfo.Default.RequiredAttributes(class)
	out := map[string]bool{}
	for _, attr := range names.AttributeNames(class) {
		declared, _ := rminfo.Default.AttributeRMType(class, attr)
		// A formal parameter is what Substitute replaces with a supplied
		// type argument.
		if bmmtype.Substitute(class+"<PROBE_ARGUMENT>", declared) == "PROBE_ARGUMENT" {
			out[attr] = slices.Contains(required, attr)
		}
	}
	return out
}

// goField is the Go field name of the RM attribute attr.
func goField(attr string) string {
	parts := strings.Split(attr, "_")
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// TestREQ107_BuiltTypeArgumentMatchesTypereg is the REQ-107 check that
// builtTypeArgument, the hand-kept type argument the generator resolves a
// class's formal generic parameter to, is the one the typereg constructor
// actually builds the class with: for each listed class, every attribute
// the BMM types by the parameter has, on the value newRMForOPTType builds,
// the Go type of the listed argument. A typereg change to the type
// argument turns this red.
func TestREQ107_BuiltTypeArgumentMatchesTypereg(t *testing.T) {
	for class, arg := range builtTypeArgument {
		t.Run(class, func(t *testing.T) {
			want, ok := typeArgumentGoTypes[arg]
			if !ok {
				t.Fatalf("builtTypeArgument[%s] = %s, which this test has no Go type for", class, arg)
			}
			built, err := newRMForOPTType(class)
			if err != nil {
				t.Fatalf("newRMForOPTType(%s): %v", class, err)
			}
			attrs := parameterTyped(t, class)
			if len(attrs) == 0 {
				t.Fatalf("%s has no attribute typed by its formal parameter, so its row resolves nothing", class)
			}
			value := reflect.ValueOf(built).Elem()
			for attr := range attrs {
				field := value.FieldByName(goField(attr))
				if !field.IsValid() {
					t.Fatalf("%T has no field for %s.%s", built, class, attr)
				}
				if got := field.Type(); got != want {
					t.Errorf("%T.%s is %v, want %v, the Go type of %s", built, goField(attr), got, want, arg)
				}
			}
		})
	}
}

// TestREQ107_BuiltTypeArgumentCoversRequiredParameters is the REQ-107 check
// that builtTypeArgument lists every class the generator can build whose
// required attribute the BMM types by a formal generic parameter: the BMM
// fill builds every required attribute, and without a row that attribute's
// type is the bare parameter, for which no default can be built. A class
// is buildable when newRMForOPTType builds it from its name, an abstract
// class through its concrete substitute (concreteFor). Optional
// parameter-typed attributes are not counted: of the classes that have one
// (DV_INTERVAL and the BASE intervals and versions), none is built over a
// type argument the generator can build a default for.
func TestREQ107_BuiltTypeArgumentCoversRequiredParameters(t *testing.T) {
	var want []string
	for _, class := range rminfo.Default.KnownRMTypes() {
		if _, err := newRMForOPTType(class); err != nil {
			continue
		}
		for _, required := range parameterTyped(t, class) {
			if required {
				want = append(want, class)
				break
			}
		}
	}
	slices.Sort(want)
	got := make([]string, 0, len(builtTypeArgument))
	for class := range builtTypeArgument {
		got = append(got, class)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("builtTypeArgument lists %v, want exactly the buildable classes with a required parameter-typed attribute %v", got, want)
	}
}
