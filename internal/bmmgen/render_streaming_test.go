package bmmgen

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// typeTag is the struct tag of the `_type` member in every wire shape.
const typeTag = "`json:\"_type\"`"

// marshalLeadsWithType reports whether the rendered marshaller chunk emits
// `_type` as the leading member. It handles every wire shape:
//
//   - flat wire struct: `_type` must be the FIRST json tag in the chunk.
//   - alias wrapper and interval-derived wrapper: the embedded alias carries
//     no tag, so a tag-order check alone would miss a reordering. The chunk
//     must declare the embed once for each json.MarshalEncode call, and every
//     such line must follow a `_type` field inside the same struct.
//
// aliasEmbed is the embedded field as the struct spells it, without type
// arguments: aliasTypeName(GoName) for the alias shape, which embeds the alias
// by value, aliasTypeName(base) for an interval-derived class, whose encoder
// wrappers embed the base's alias by value, or "" for the flat shape. A chunk
// that spells the embed any other way, or encodes through a value that does
// not embed it, reads as not leading, so a change of spelling turns the census
// red instead of passing it unchecked.
func marshalLeadsWithType(chunk, aliasEmbed string) bool {
	tPos := strings.Index(chunk, typeTag)
	if tPos < 0 {
		return false
	}
	// No other json tag may precede `_type`.
	if first := strings.Index(chunk, "`json:\""); first >= 0 && first < tPos {
		return false
	}
	if aliasEmbed == "" {
		return true
	}
	embeds := embedLineOffsets(chunk, aliasEmbed)
	if len(embeds) == 0 || len(embeds) != strings.Count(chunk, "json.MarshalEncode(") {
		return false
	}
	for _, ePos := range embeds {
		open := strings.LastIndex(chunk[:ePos], "struct {")
		if open < 0 || !strings.Contains(chunk[open:ePos], typeTag) {
			return false
		}
	}
	return true
}

// embedLineOffsets returns the offset of every line of chunk that declares
// embed as an embedded field: the line, without its indentation, is embed
// alone or embed followed by a type-argument list. The alias declaration
// `type rawX X` and the conversion `rawX(x)` in a composite literal never
// match.
func embedLineOffsets(chunk, embed string) []int {
	var offsets []int
	off := 0
	for line := range strings.Lines(chunk) {
		field := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(field, embed); ok &&
			(rest == "" || strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]")) {
			offsets = append(offsets, off)
		}
		off += len(line)
	}
	return offsets
}

// TestRenderMarshalLeadsWithType pins the REQ-052 SHOULD that `_type` is the
// leading member. `_type` is never a declared BMM property, so a class's fields
// are always "reordered" relative to it; the generator INJECTS it first in the
// anonymous wrapper (alias shape) or as the flat wire struct's first field. The
// census below renders every concrete class and asserts each marshaller leads
// with `_type`, regardless of the class's own field order.
//
// Can-fail control: [marshalLeadsWithType] is proved discriminating on
// deliberately broken renderings, so the census is not trivially satisfied:
// an alias embed ahead of `_type`, an alias embedded through a pointer
// instead of by value, an alias held in a named field with no embed line at
// all, a second wrapper reordered behind a correct first one, an
// interval-derived wrapper reordered, an encode through the interval wire
// struct instead of a wrapper, and a flat `_type` tag behind another tag.
// Moving the `_type` field after the alias in renderMarshalAlias or
// renderMarshalIntervalDerived, after another field in renderMarshalFlat, or
// changing how the alias is embedded turns the census red.
func TestRenderMarshalLeadsWithType(t *testing.T) {
	// Prove the witness discriminates on every shape. Each broken rendering
	// must read as NOT leading with `_type`; without them, the census could be
	// trivially satisfied for a shape the witness happens not to exercise. The
	// well-formed generic wrappers must still read as leading, so the type
	// arguments, the field-keyed literal and the decode-side wire struct, which
	// embeds the base through a pointer, do not throw the witness off.
	const intervalWireDecl = "type jsonWireX[T any] struct {\n\tType string `json:\"_type\"`\n\t*rawI[T]\n}\n\n"
	for _, tc := range []struct {
		name       string
		chunk      string
		aliasEmbed string
		want       bool
	}{
		{
			name:       "alias embed precedes _type",
			chunk:      "type rawX X\n\treturn json.MarshalEncode(enc, &struct {\n\t\trawX\n\t\tType string `json:\"_type\"`\n\t}{rawX(x), \"X\"}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawX",
			want:       false,
		},
		{
			name:       "alias embedded through a pointer, not by value",
			chunk:      "type rawX X\n\treturn json.MarshalEncode(enc, &struct {\n\t\tType string `json:\"_type\"`\n\t\t*rawX\n\t}{\"X\", rawX(x)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawX",
			want:       false,
		},
		{
			name:       "alias held in a named field, no embed line",
			chunk:      "type rawX X\n\treturn json.MarshalEncode(enc, &struct {\n\t\tType string `json:\"_type\"`\n\t\tValue rawX\n\t}{\"X\", rawX(x)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawX",
			want:       false,
		},
		{
			name: "second wrapper embeds the alias before _type",
			chunk: "type rawX[T any] X[T]\n" +
				"\t\treturn json.MarshalEncode(enc, &struct {\n\t\t\tType string `json:\"_type\"`\n\t\t\trawX[T]\n\t\t}{Type: \"X\", rawX: rawX[T](x)}, typereg.MarshalOptions(enc))\n" +
				"\treturn json.MarshalEncode(enc, &struct {\n\t\trawX[T]\n\t\tType string `json:\"_type\"`\n\t}{Type: \"X\", rawX: rawX[T](x)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawX",
			want:       false,
		},
		{
			name:       "flat _type tag follows another tagged field",
			chunk:      "type jsonWireX struct {\n\tValue string `json:\"value\"`\n\tClass string `json:\"_type\"`\n}",
			aliasEmbed: "",
			want:       false,
		},
		{
			name:       "generic alias embed follows _type",
			chunk:      "type rawX[T any] X[T]\n\treturn json.MarshalEncode(enc, &struct {\n\t\tType string `json:\"_type\"`\n\t\trawX[T]\n\t\tLower struct{} `json:\"lower,omitzero\"`\n\t}{Type: \"X\", rawX: rawX[T](x)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawX",
			want:       true,
		},
		{
			name: "interval-derived wrapper embeds the base before _type",
			chunk: intervalWireDecl +
				"\t\treturn json.MarshalEncode(enc, &struct {\n\t\t\tType string `json:\"_type\"`\n\t\t\trawI[T]\n\t\t\tLower struct{} `json:\"lower,omitzero\"`\n\t\t}{Type: \"X\", rawI: rawI[T](x.I)}, typereg.MarshalOptions(enc))\n" +
				"\treturn json.MarshalEncode(enc, &struct {\n\t\trawI[T]\n\t\tType string `json:\"_type\"`\n\t}{Type: \"X\", rawI: rawI[T](x.I)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawI",
			want:       false,
		},
		{
			name: "interval-derived arm encodes through the wire struct, not a wrapper",
			chunk: intervalWireDecl +
				"\t\treturn json.MarshalEncode(enc, &struct {\n\t\t\tType string `json:\"_type\"`\n\t\t\trawI[T]\n\t\t\tLower struct{} `json:\"lower,omitzero\"`\n\t\t}{Type: \"X\", rawI: rawI[T](x.I)}, typereg.MarshalOptions(enc))\n" +
				"\treturn json.MarshalEncode(enc, w, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawI",
			want:       false,
		},
		{
			name: "interval-derived wrappers embed the base after _type",
			chunk: intervalWireDecl +
				"\t\treturn json.MarshalEncode(enc, &struct {\n\t\t\tType string `json:\"_type\"`\n\t\t\trawI[T]\n\t\t\tLower struct{} `json:\"lower,omitzero\"`\n\t\t}{Type: \"X\", rawI: rawI[T](x.I)}, typereg.MarshalOptions(enc))\n" +
				"\treturn json.MarshalEncode(enc, &struct {\n\t\tType string `json:\"_type\"`\n\t\trawI[T]\n\t}{Type: \"X\", rawI: rawI[T](x.I)}, typereg.MarshalOptions(enc))\n",
			aliasEmbed: "rawI",
			want:       true,
		},
	} {
		if got := marshalLeadsWithType(tc.chunk, tc.aliasEmbed); got != tc.want {
			t.Fatalf("marshalLeadsWithType(%s) = %v, want %v: the witness does not discriminate, so the census below proves nothing", tc.name, got, tc.want)
		}
	}

	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var checked int
	for _, file := range plan.Files {
		for _, pc := range concreteClassesIn(file) {
			chunk, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON %s: %v", pc.BMMName, err)
			}
			shape, base, err := classJSONWireShape(plan, pc)
			if err != nil {
				t.Fatalf("classJSONWireShape %s: %v", pc.BMMName, err)
			}
			var aliasEmbed string
			switch shape {
			case shapeFlat:
			case shapeAlias, shapeIntervalAlias:
				aliasEmbed = aliasTypeName(pc.GoName)
			case shapeIntervalDerived:
				aliasEmbed = aliasTypeName(base.GoName)
			case shapeIntervalBase:
				t.Fatalf("%s takes the shape of the abstract BASE Interval, which writes no `_type`, yet the plan lists it as concrete", pc.BMMName)
			default:
				t.Fatalf("%s takes wire shape %d, which the census has no witness for", pc.BMMName, shape)
			}
			if !marshalLeadsWithType(chunk, aliasEmbed) {
				t.Errorf("%s marshaller does not lead with `_type`: the discriminator must be the first member (REQ-052)", pc.BMMName)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("census rendered only %d concrete classes; expected the full RM inventory", checked)
	}
}

// TestRenderIntervalEncodersCopyTheReceiverOnce pins how the interval
// encoders reach BASE Interval's members: every value they encode is a wrapper
// that embeds the base's method-free alias by value, and no encoder takes the
// address of its receiver. Taking it moves the value receiver to the heap, an
// allocation per encoded value beside the wrapper's own. BASE Interval writes
// no `_type`, so the census above does not reach its encoder; this test checks
// it beside the three classes that embed it.
func TestRenderIntervalEncodersCopyTheReceiverOnce(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	for _, class := range []string{"Interval", "Point_interval", "Proper_interval", "DV_INTERVAL"} {
		t.Run(class, func(t *testing.T) {
			pc := plan.Classes[class]
			if pc == nil {
				t.Fatalf("%s is not in the RM plan", class)
			}
			shape, base, err := classJSONWireShape(plan, pc)
			if err != nil {
				t.Fatalf("classJSONWireShape(%s): %v", class, err)
			}
			switch shape {
			case shapeIntervalBase:
				base = pc
			case shapeIntervalDerived:
			case shapeAlias, shapeFlat, shapeIntervalAlias:
				t.Fatalf("classJSONWireShape(%s) = %d, want the BASE Interval shape or the interval-derived shape", class, shape)
			default:
				t.Fatalf("classJSONWireShape(%s) = %d, a shape this test has no case for", class, shape)
			}
			chunk, err := renderMarshalJSON(plan, pc)
			if err != nil {
				t.Fatalf("renderMarshalJSON(%s): %v", class, err)
			}
			_, body, ok := strings.Cut(chunk, ") MarshalJSONTo(enc *jsontext.Encoder) error {\n")
			if !ok {
				t.Fatalf("no MarshalJSONTo in the %s chunk:\n%s", class, chunk)
			}
			recv := jsonmarReceiverName(pc.GoName)
			if addr := regexp.MustCompile(`&` + regexp.QuoteMeta(recv) + `\b`); addr.MatchString(body) {
				t.Errorf("%s MarshalJSONTo takes the address of its receiver %q, which moves it to the heap:\n%s", class, recv, body)
			}
			calls := strings.Count(body, "json.MarshalEncode(")
			embeds := len(embedLineOffsets(body, aliasTypeName(base.GoName)))
			if calls == 0 || embeds != calls {
				t.Errorf("%s MarshalJSONTo makes %d json.MarshalEncode calls and embeds %s by value %d times, want one by-value embed per call:\n%s", class, calls, aliasTypeName(base.GoName), embeds, body)
			}
		})
	}
}

// TestRenderFlatDecodeDropPropertyLosesCopy is the generator-side can-fail
// control for TestRoundTripStructuralEquivalence
// (openehr/serialize/canjson/roundtrip_test.go): a flat-shape decode copies
// every wire field back to the receiver, so dropping one property from the plan
// drops its copy: the field would then be silently lost on the round trip.
//
// DV_CODED_TEXT is flat-shape (it embeds the marshaler-bearing DVText, so it
// cannot use the alias shape, ruling R19), which is exactly the shape whose
// per-field copy a dropped assignment would break.
func TestRenderFlatDecodeDropPropertyLosesCopy(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	pc, ok := plan.Classes["DV_CODED_TEXT"]
	if !ok {
		t.Fatal("DV_CODED_TEXT not in plan")
	}
	if !embedsMarshalerBearingConcrete(plan, pc) {
		t.Fatal("DV_CODED_TEXT is not flat-shape; this control needs a class whose decode copies fields")
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields: %v", err)
	}
	if len(fields) < 2 {
		t.Fatalf("DV_CODED_TEXT has %d effective fields; need at least 2 to drop one", len(fields))
	}
	dropped := fields[len(fields)-1]
	droppedCopy := "d." + FieldName(dropped.Prop.PropertyName()) + " = wire." + FieldName(dropped.Prop.PropertyName())

	full, err := renderUnmarshalJSON(plan, pc, fields)
	if err != nil {
		t.Fatalf("renderUnmarshalJSON (full): %v", err)
	}
	if !strings.Contains(full, droppedCopy) {
		t.Fatalf("baseline decode does not copy %q; the control cannot demonstrate the loss", droppedCopy)
	}

	trimmed, err := renderUnmarshalJSON(plan, pc, fields[:len(fields)-1])
	if err != nil {
		t.Fatalf("renderUnmarshalJSON (dropped): %v", err)
	}
	if strings.Contains(trimmed, droppedCopy) {
		t.Errorf("dropping %s from the plan still emitted its copy %q: a lost property would go unnoticed on the round trip", dropped.Prop.PropertyName(), droppedCopy)
	}
}

// TestRenderUnmarshalPassesDeclaredTypeField pins, on every concrete class the
// RM plan emits, that the rendered UnmarshalJSONFrom hands typereg.DecodeInto
// the wire value's own declared `_type` field as gotType (REQ-052, ADR 0022):
// `&wire.Class` on the flat shape, `&w.Type` on the alias shape and
// `&wire.Type` on the interval wire struct. The
// whole-value `_type` guard reads that field after the single decode, so any
// other pointer (a fresh `new(string)`, say) would leave it reading an empty
// discriminator and accept a mislabelled body silently. The runtime twin is
// TestUnmarshalTypeMismatchCensus in openehr/serialize/canjson.
func TestRenderUnmarshalPassesDeclaredTypeField(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var checked, concrete int
	for _, file := range plan.Files {
		classes := concreteClassesIn(file)
		concrete += len(classes)
		for _, pc := range classes {
			fields, err := effectiveFields(plan, pc)
			if err != nil {
				t.Fatalf("effectiveFields %s: %v", pc.BMMName, err)
			}
			chunk, err := renderUnmarshalJSON(plan, pc, fields)
			if err != nil {
				t.Fatalf("renderUnmarshalJSON %s: %v", pc.BMMName, err)
			}
			shape, want := "alias", `typereg.DecodeInto(dec, "`+pc.BMMName+`", &w, &w.Type)`
			if embedsMarshalerBearingConcrete(plan, pc) {
				shape, want = "flat", `typereg.DecodeInto(dec, "`+pc.BMMName+`", &wire, &wire.Class)`
			}
			if got, _, err := classJSONWireShape(plan, pc); err != nil {
				t.Fatalf("classJSONWireShape %s: %v", pc.BMMName, err)
			} else if got == shapeIntervalDerived {
				shape, want = "interval", `typereg.DecodeInto(dec, "`+pc.BMMName+`", wire, &wire.Type)`
			}
			if !strings.Contains(chunk, want) {
				t.Errorf("%s (%s shape) decoder does not contain %s: the `_type` guard must read the wire value's declared field", pc.BMMName, shape, want)
			}
			checked++
		}
	}
	// Every earlier failure is fatal, so checked == concrete holds by
	// construction; the floor is what catches a plan that silently shrank.
	if concrete < 100 || checked != concrete {
		t.Errorf("census rendered %d of the %d concrete classes the plan yields; want all of them, and the full RM inventory (at least 100)", checked, concrete)
	}
}
