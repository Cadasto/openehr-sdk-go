package bmmgen

import (
	"fmt"
	"strings"
	"testing"
)

// TestREQ052DirectDecodeCoversEveryShape renders the decoder of every codec
// class of the RM and AOM 1.4 targets and pins that each wire shape the
// wrapper decode handles has a direct read, so no class falls back to the
// wrapper for good: the alias shape, an Any member's decoder among them, the
// flat shape, the interval wire struct and BASE Interval, generic classes
// and members typed by a type parameter included. Each UnmarshalJSONFrom
// refuses a nil receiver, then takes the wrapper only where
// typereg.DirectDecode says so; its readJSONMembers has one case for `_type`,
// unless the class is BASE Interval, which reads none, and one for each
// member the wrapper knows, reading it as its shape needs; and the wrapper,
// unmarshalJSONWrapped, still decodes through typereg.DecodeInto. The census
// at the end fails when a shape stops reaching the renderer, so a shape added
// later is either covered here or named.
func TestREQ052DirectDecodeCoversEveryShape(t *testing.T) {
	t.Parallel()
	census := map[string]int{}
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan := buildPlanWithITSXML(t, target)
		idx := directTypeIndex(plan)
		for _, f := range plan.Files {
			classes, err := codecClassesIn(plan, f)
			if err != nil {
				t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
			}
			for _, pc := range classes {
				fields, err := effectiveFields(plan, pc)
				if err != nil {
					t.Fatalf("effectiveFields(%s): %v", pc.BMMName, err)
				}
				src, err := renderUnmarshalJSON(plan, pc, fields)
				if err != nil {
					t.Fatalf("renderUnmarshalJSON(%s): %v", pc.BMMName, err)
				}
				shape, base, err := classJSONWireShape(plan, pc)
				if err != nil {
					t.Fatal(err)
				}
				flagged, err := intervalFlagged(fields)
				if err != nil {
					t.Fatal(err)
				}
				anyMembers, err := jsonAnyMembers(plan, pc, fields)
				if err != nil {
					t.Fatal(err)
				}
				members, err := directMembers(plan, idx, pc, shape, base)
				if err != nil {
					t.Fatal(err)
				}
				for _, kind := range checkDirectDecode(t, pc, src, shape, flagged, anyMembers, members) {
					census[kind]++
				}
			}
		}
	}
	t.Logf("decoders by kind: %v", census)
	for _, kind := range []string{"alias", "flat", "interval wire struct", "BASE Interval", "Any member", "generic", "type parameter member"} {
		if census[kind] == 0 {
			t.Errorf("no class of shape %q reached the renderer; census %v", kind, census)
		}
	}
	if census["alias"]+census["flat"]+census["interval wire struct"]+census["BASE Interval"] < 150 {
		t.Errorf("rendered %v decoders, want every codec class of both targets", census)
	}
}

// checkDirectDecode pins the decoder src renders for pc, of wire shape
// shape, as [TestREQ052DirectDecodeCoversEveryShape] says, and returns the
// kinds of decoder it counts as.
func checkDirectDecode(t *testing.T, pc *PlannedClass, src string, shape jsonWireShape, flagged bool, anyMembers []jsonAnyMember, members []directMember) []string {
	t.Helper()
	recv := jsonmarReceiverName(pc.GoName)
	direct, wrapped, ok := strings.Cut(src, ") unmarshalJSONWrapped(dec *jsontext.Decoder) error {\n")
	if !ok {
		t.Errorf("%s has no wrapper decode:\n%s", pc.BMMName, src)
		return nil
	}
	if !strings.Contains(wrapped, "typereg.DecodeInto(dec, ") {
		t.Errorf("%s wrapper decode does not go through typereg.DecodeInto:\n%s", pc.BMMName, wrapped)
	}
	gate := fmt.Sprintf("\t\treturn fmt.Errorf(\"canjson: %s: %%w\", typereg.ErrNilReceiver)\n\t}\n"+
		"\tif !typereg.DirectDecode(dec) {\n\t\treturn %s.unmarshalJSONWrapped(dec)\n\t}\n", pc.BMMName, recv)
	if !strings.Contains(direct, gate) {
		t.Errorf("%s UnmarshalJSONFrom does not refuse a nil receiver and then take the wrapper where DirectDecode says so:\n%s", pc.BMMName, direct)
	}
	_, reader, ok := strings.Cut(direct, ") readJSONMembers(dec *jsontext.Decoder")
	if !ok {
		t.Errorf("%s has no readJSONMembers:\n%s", pc.BMMName, direct)
		return nil
	}
	readsType := shape != shapeIntervalBase
	want := len(members)
	if readsType {
		want++
		if typeCase := fmt.Sprintf("\t\tcase \"_type\":\n\t\t\tgot, err = typereg.DecodeTypeMember(dec, %q)\n", pc.BMMName); !strings.Contains(reader, typeCase) {
			t.Errorf("%s readJSONMembers does not read `_type`:\n%s", pc.BMMName, reader)
		}
	} else if strings.Contains(reader, "case \"_type\":") {
		t.Errorf("%s readJSONMembers reads a `_type` the class does not declare:\n%s", pc.BMMName, reader)
	}
	if got := strings.Count(reader, "\t\tcase "); got != want {
		t.Errorf("%s readJSONMembers has %d cases, want %d: `_type` and the %d members the wrapper knows:\n%s", pc.BMMName, got, want, len(members), reader)
	}
	if !strings.Contains(reader, "\t\tdefault:\n\t\t\terr = dec.SkipValue()\n") {
		t.Errorf("%s readJSONMembers does not skip a member it does not know:\n%s", pc.BMMName, reader)
	}
	anyByName := map[string]jsonAnyMember{}
	for _, m := range anyMembers {
		anyByName[m.name] = m
	}
	kinds := []string{}
	for _, m := range members {
		read := fmt.Sprintf("json.UnmarshalDecode(dec, &%s.%s)", recv, m.field)
		switch a, isAny := anyByName[m.name]; {
		case isAny:
			read = fmt.Sprintf("json.UnmarshalDecode(dec, &%s{To: &%s.%s", a.decoder, recv, m.field)
			kinds = append(kinds, "Any member")
		case flagged && isIntervalFlag(m.name):
			read = fmt.Sprintf("decodeIntervalFlagJSON(dec, &%s.%s, &seen.%s)", recv, m.field, seenFlagField(m.name))
		case flagged && isIntervalBound(m.name):
			read = fmt.Sprintf("decodeIntervalBoundJSON(dec, &%s.%s, &seen.%s)", recv, m.field, seenFlagField(m.name))
		}
		if m.kind == directParam {
			kinds = append(kinds, "type parameter member")
		}
		if c := fmt.Sprintf("\t\tcase %q:\n\t\t\terr = %s", m.name, read); !strings.Contains(reader, c) {
			t.Errorf("%s readJSONMembers does not read %q as %s:\n%s", pc.BMMName, m.name, read, reader)
		}
	}
	if strings.Contains(direct, "[T") {
		kinds = append(kinds, "generic")
	}

	switch {
	case flagged:
		settle := strings.Index(direct, intervalFlagSettle("seen", recv))
		done := strings.Index(direct, "\t\treturn typereg.DecodeDone(")
		if settle < 0 || done < 0 || done > settle || !strings.Contains(direct, "\tvar seen intervalFlagsSeen\n") {
			t.Errorf("%s UnmarshalJSONFrom does not record the flags in a local and give the defaults after a read that succeeds:\n%s", pc.BMMName, direct)
		}
		if shape == shapeIntervalBase {
			kinds = append(kinds, "BASE Interval")
		} else {
			kinds = append(kinds, "interval wire struct")
		}
	case shape == shapeFlat:
		restore := fmt.Sprintf("\tsaved := *%s\n\t*%s = %s", recv, recv, pc.GoName)
		back := fmt.Sprintf("\tif err != nil || got != \"\" {\n\t\t*%s = saved\n\t}\n", recv)
		if !strings.Contains(direct, restore) || !strings.Contains(direct, back) {
			t.Errorf("%s UnmarshalJSONFrom does not start from the zero value and put the receiver back on a failure, as its wrapper does:\n%s", pc.BMMName, direct)
		}
		kinds = append(kinds, "flat")
	case shape == shapeAlias || shape == shapeIntervalAlias:
		if strings.Contains(direct, "saved") || strings.Contains(direct, "seen") {
			t.Errorf("%s decodes in place through its alias, but its direct read saves the receiver or records flags:\n%s", pc.BMMName, direct)
		}
		kinds = append(kinds, "alias")
	default:
		t.Errorf("%s takes wire shape %d, which the direct decoder does not name", pc.BMMName, shape)
	}
	return kinds
}

// TestREQ052DirectDecodeRefusesWhatItCannotRead pins that the generator
// refuses a member list the direct read would read otherwise than the
// wrapper: a member with no JSON name, or named `_type`, which the reader
// takes as the discriminator, an Any member missing from the list, which the
// wrapper would decode and the direct read skip, and one that decodes into
// another field than the list names.
func TestREQ052DirectDecodeRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	item := jsonAnyMember{field: "Item", name: "item", decoder: "xmlschema.AnyJSON"}
	for _, tc := range []struct {
		name       string
		members    []directMember
		anyMembers []jsonAnyMember
	}{
		{"a member with no JSON name", []directMember{{name: "", field: "X"}}, nil},
		{"a member named _type", []directMember{{name: "_type", field: "Type"}}, nil},
		{"an Any member not among the members", []directMember{{name: "other", field: "Other"}}, []jsonAnyMember{item}},
		{"an Any member read into another field", []directMember{{name: "item", field: "Other"}}, []jsonAnyMember{item}},
	} {
		if got, err := decodeMembersOf("X", tc.members, tc.anyMembers, false); err == nil {
			t.Errorf("decodeMembersOf(%s) = %+v, want a refusal", tc.name, got)
		}
	}
	got, err := decodeMembersOf("X", []directMember{{name: "item", field: "Item"}, {name: "lower", field: "Lower"}, {name: "lower_included", field: "LowerIncluded"}}, []jsonAnyMember{item}, true)
	if err != nil {
		t.Fatalf("decodeMembersOf: %v", err)
	}
	if len(got) != 3 || got[0].read != readAny || got[1].read != readBound || got[2].read != readFlag {
		t.Errorf("decodeMembersOf read %+v, want the Any member, the bound and the flag through their own readers", got)
	}
}
