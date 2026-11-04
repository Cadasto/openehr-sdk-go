package validation

import (
	"reflect"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// rmClassNames returns, sorted, the names of the classes the type registry
// decodes whose Go type is in the RM package. The registry also holds the
// classes of any other generated package the test binary links, such as the
// AOM 1.4 model the compile step reads, which the floor does not validate
// (REQ-171: the floor covers every concrete RM class the registry knows).
func rmClassNames() []string {
	rmPackage := reflect.TypeFor[rm.Integer]().PkgPath()
	var names []string
	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			continue
		}
		class := reflect.TypeOf(ctor())
		for class.Kind() == reflect.Pointer {
			class = class.Elem()
		}
		if class.PkgPath() == rmPackage {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}
