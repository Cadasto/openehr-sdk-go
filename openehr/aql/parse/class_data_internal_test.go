package parse

// class_data_internal_test.go: carriesData compares classData's copy of a
// ClassExpr, since ClassExpr itself is not comparable. A field added to
// ClassExpr must be copied there, or set aside here as a read-side
// derivation that emission never reads (REQ-119 § Emission ignores the
// structured model).

import (
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// setAside are the ClassExpr fields carriesData does not compare.
var setAside = map[string]bool{"Pos": true, "ParsedPredicate": true, "Archetypes": true}

func TestClassDataCoversEveryClassExprField(t *testing.T) {
	t.Parallel()
	ct := reflect.TypeFor[ClassExpr]()
	for i := range ct.NumField() {
		f := ct.Field(i)
		if setAside[f.Name] {
			continue
		}
		var ce ClassExpr
		v := reflect.ValueOf(&ce).Elem().Field(i)
		switch v.Kind() {
		case reflect.String:
			v.SetString("x")
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Pointer:
			v.Set(reflect.New(f.Type.Elem()))
		default:
			t.Fatalf("ClassExpr.%s is a %s: copy it in classData and give it a non-zero value here, or set it aside", f.Name, f.Type)
		}
		if !ce.carriesData() {
			t.Errorf("ClassExpr{%s: non-zero}.carriesData() = false; want true: classData does not copy %s", f.Name, f.Name)
		}
	}
}

func TestClassDataSetsTheReadSideFieldsAside(t *testing.T) {
	t.Parallel()
	ce := ClassExpr{
		Pos:             Position{Line: 1, Col: 1},
		ParsedPredicate: aql.NodeIDPredicate{ID: "at0001"},
		Archetypes:      []string{"openEHR-EHR-OBSERVATION.lab_result.v1"},
	}
	if ce.carriesData() {
		t.Error("carriesData() = true for a ClassExpr holding only its position and read-side fields; want false")
	}
}
