package bmm

import "testing"

func TestPropertyOrderPreserved(t *testing.T) {
	raw := []byte(`{
		"name": "Demo",
		"properties": {
			"zebra": {"_type": "P_BMM_SINGLE_PROPERTY_OPEN", "name": "zebra", "type": "String"},
			"alpha": {"_type": "P_BMM_SINGLE_PROPERTY_OPEN", "name": "alpha", "type": "String"}
		}
	}`)
	sc, err := decodeSimpleClass(raw, "test")
	if err != nil {
		t.Fatalf("decodeSimpleClass: %v", err)
	}
	want := []string{"zebra", "alpha"}
	if len(sc.PropertyOrder) != len(want) {
		t.Fatalf("PropertyOrder len = %d, want %d", len(sc.PropertyOrder), len(want))
	}
	for i, w := range want {
		if sc.PropertyOrder[i] != w {
			t.Fatalf("PropertyOrder[%d] = %q, want %q (full %v)", i, sc.PropertyOrder[i], w, sc.PropertyOrder)
		}
	}
}
