package validation

// class_facts_internal_test.go: REQ-171 § Surface — what the walk reads of
// the attributes of each class, built once per class, equals what
// rminfo.Default answers when asked directly.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// runtimeFacts returns what rminfo.Default says of the attribute name of the
// bare RM class class, each answer asked of it directly.
func runtimeFacts(t *testing.T, class, name string) attributeFacts {
	t.Helper()
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default lists no attributes")
	}
	if !slices.Contains(lister.AttributeNames(class), name) {
		t.Fatalf("rminfo.Default does not list %s on %s", name, class)
	}
	absence, ok := rminfo.Default.(rminfo.AbsenceReporter)
	if !ok {
		t.Fatal("rminfo.Default reports no absence reasons")
	}
	f := attributeFacts{
		Name:        name,
		NonStorable: rminfo.IsNonStorableAttr(class, name),
		Mandatory:   slices.Contains(rminfo.Default.RequiredAttributes(class), name),
	}
	f.RMType, f.Typed = rminfo.Default.AttributeRMType(class, name)
	f.Container, _ = rminfo.Default.IsContainer(class, name)
	f.PrimitiveItems = absence.AbsenceReason(f.RMType) == rminfo.AbsencePrimitive
	f.Hash = f.RMType == "Hash"
	return f
}

// REQ-171 § Surface — the facts the walk reads of every attribute of every
// class the class metadata knows, built once per class into the table it
// reads every node by, equal what rminfo.Default answers at run time, in
// the order it lists them; a class it does not know has none.
func TestREQ171_ClassFactsEqualTheRuntimeAnswers(t *testing.T) {
	t.Parallel()
	lister, _ := rminfo.Default.(rminfo.AttributeLister)
	for _, class := range rminfo.Default.KnownRMTypes() {
		var want []attributeFacts
		for _, name := range lister.AttributeNames(class) {
			want = append(want, runtimeFacts(t, class, name))
		}
		if got := classFacts(class); !slices.Equal(got, want) {
			t.Errorf("classFacts(%s) = %+v, want %+v", class, got, want)
		}
		if got := classAttributes[class]; got == nil || !slices.Equal(got.listed, want) {
			t.Errorf("classAttributes[%s] = %+v, want the table listing %+v", class, got, want)
		}
	}
	if got := classFacts("NO_SUCH_CLASS"); got != nil {
		t.Errorf("classFacts(NO_SUCH_CLASS) = %+v, want none", got)
	}
}
