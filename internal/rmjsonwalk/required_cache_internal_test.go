package rmjsonwalk

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestRequiredListsCacheOnlyRMClasses pins what keeps the cache of required
// attribute lists bounded: a walk caches the list of a class it resolved, and
// a "_type" that names no RM class resolves to none, so the cache never takes
// a name a document wrote. The Default model lives for the whole process and
// every validated document passes through it, so a cache keyed by "_type"
// values as written would grow with every distinct misspelling or invented
// class a caller ever sends.
//
// The document mixes RM classes with names that are not, so the cache is in
// use: a walk that stopped caching would leave it empty and prove nothing,
// which the premise check catches.
func TestRequiredListsCacheOnlyRMClasses(t *testing.T) {
	m, err := New(rminfo.Default)
	if err != nil {
		t.Fatalf("New(rminfo.Default): %v", err)
	}
	const doc = `{"_type":"COMPOSITION",
		"name":{"_type":"DV_TEXT_INVENTED","value":"x"},
		"language":{"_type":"CODE_PHRASE"},
		"uid":{"_type":"not_a_class"},
		"content":[{"_type":"NO_SUCH_ENTRY"},{"_type":"OBSERVATION"},{"_type":"observation"},{"_type":""}]}`
	missing := 0
	if err := m.Walk([]byte(doc), Root{Type: "COMPOSITION"}, Hooks{Missing: func(*reportpath.Node) error {
		missing++
		return nil
	}}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if missing == 0 {
		t.Fatal("premise gone: the walk reported no missing member, so it may not have read any required list")
	}

	cached := map[string]bool{}
	m.lists.Range(func(k, _ any) bool {
		name, _ := k.(string)
		cached[name] = true
		if !m.IsClass(name) {
			t.Errorf("the required-list cache holds %q, which names no RM class", k)
		}
		return true
	})
	for _, class := range []string{"COMPOSITION", "CODE_PHRASE", "OBSERVATION"} {
		if !cached[class] {
			t.Errorf("premise gone: the required-list cache holds no list for %s, which the document writes", class)
		}
	}
}
