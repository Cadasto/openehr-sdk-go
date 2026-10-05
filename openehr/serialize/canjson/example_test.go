package canjson_test

import (
	"fmt"
	"log"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// Decode one ELEMENT from canonical JSON. The element's value slot can hold
// any DATA_VALUE, so the decoder reads its "_type" member and builds the
// matching Go type, here a *rm.DVQuantity.
func ExampleUnmarshal() {
	data := []byte(`{
		"_type": "ELEMENT",
		"archetype_node_id": "at0004",
		"name": {"_type": "DV_TEXT", "value": "Systolic"},
		"value": {
			"_type": "DV_QUANTITY",
			"magnitude": 120,
			"units": "mm[Hg]"
		}
	}`)

	var element rm.Element
	if err := canjson.Unmarshal(data, &element); err != nil {
		log.Fatal(err)
	}

	fmt.Println("node:", element.ArchetypeNodeID)
	fmt.Println("name:", element.Name.GetValue())

	quantity, ok := element.Value.(*rm.DVQuantity)
	if !ok {
		log.Fatalf("value is %T, want *rm.DVQuantity", element.Value)
	}
	fmt.Println("value:", quantity.Magnitude, quantity.Units)
	// Output:
	// node: at0004
	// name: Systolic
	// value: 120 mm[Hg]
}

// Encode a DV_QUANTITY and decode it back. The encoder writes "_type" first
// and leaves out optional members that are not set, such as precision here.
// The order of the other members is not part of the format.
func ExampleMarshal() {
	quantity := rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}

	data, err := canjson.Marshal(&quantity)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))

	var decoded rm.DVQuantity
	if err := canjson.Unmarshal(data, &decoded); err != nil {
		log.Fatal(err)
	}
	fmt.Println("decoded:", decoded.Magnitude, decoded.Units)
	// Output:
	// {"_type":"DV_QUANTITY","magnitude":120,"units":"mm[Hg]"}
	// decoded: 120 mm[Hg]
}
