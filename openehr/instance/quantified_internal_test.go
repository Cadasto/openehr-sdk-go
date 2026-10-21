package instance

import (
	"errors"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// accuracyRoot is a DV_QUANTITY template root whose accuracy the C_REAL
// body constrains and whose accuracy_is_percent the C_BOOLEAN body does.
func accuracyRoot(t *testing.T, realBody, booleanBody string) *nodeAndPath {
	t.Helper()
	opt := visitRootOPT("DV_QUANTITY",
		visitSingle("accuracy", `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>REAL</rm_type_name><node_id></node_id>`+
			`<item xsi:type="C_REAL">`+realBody+`</item></children>`)+
			visitSingle("accuracy_is_percent", `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>BOOLEAN</rm_type_name><node_id></node_id>`+
				`<item xsi:type="C_BOOLEAN">`+booleanBody+`</item></children>`))
	root, _ := visitAttribute(t, opt, "accuracy")
	return &nodeAndPath{root: root, path: bindingChild(root, "accuracy").AQLPath()}
}

// TestREQ107_PercentageAccuracyCandidates pins the order in which
// percentageAccuracy tries the accuracies it narrows a percentage to
// (§ Precedence 3): the accuracy the walk wrote where it is more than 0
// and at most 100, then 100 where that accuracy is above it, the members
// of the C_REAL's list, 1, 100, and the middle of the part of the range
// from 0 to 100, each where the C_REAL accepts it; and none where no such
// value is accepted.
func TestREQ107_PercentageAccuracyCandidates(t *testing.T) {
	const (
		trueOnly = `<true_valid>true</true_valid><false_valid>false</false_valid>`
		upTo200  = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>200</upper></range>`
		upToHalf = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>0.5</upper></range>`
	)
	cases := []struct {
		name     string
		realBody string
		current  float64
		want     float64
		none     bool
	}{
		{name: "a percentage is kept", realBody: upTo200, current: 37, want: 37},
		{name: "above 100, 100 inside the range", realBody: upTo200, current: 150, want: 100},
		{name: "0, 1 inside the range", realBody: upTo200, current: 0, want: 1},
		{name: "0, the first percentage the list holds", realBody: `<list>0</list><list>120</list><list>7</list><list>3</list>`, current: 0, want: 7},
		{name: "0, only the middle of the range is a percentage", realBody: upToHalf, current: 0, want: 0.25},
		{name: "a list with no percentage", realBody: `<list>150</list><list>0</list>`, current: 150, none: true},
	}
	for _, tc := range cases {
		n := accuracyRoot(t, tc.realBody, trueOnly)
		got, found := percentageAccuracy(bindingChild(n.root, "accuracy"), tc.current)
		switch {
		case tc.none && found:
			t.Errorf("%s: percentageAccuracy(%v) = %v, want none", tc.name, tc.current, got)
		case !tc.none && (!found || got != tc.want):
			t.Errorf("%s: percentageAccuracy(%v) = %v, %t, want %v", tc.name, tc.current, got, found, tc.want)
		}
	}
	if got, found := percentageAccuracy(nil, -1); !found || got != 1 {
		t.Errorf("percentageAccuracy(no OPT node, -1) = %v, %t, want 1", got, found)
	}
}

// nodeAndPath is a compiled template root and the OPT path of its
// accuracy leaf.
type nodeAndPath struct {
	root *tcimpl.CompiledNode
	path string
}

// TestREQ107_SettleAccuracyLeavesWhatNoRuleBinds pins what settleAccuracy
// changes (§ Precedence 3): a percentage accuracy outside more than 0 and
// at most 100 is narrowed; where the OPT admits no percentage,
// accuracy_is_percent becomes false where the OPT admits false, and the
// call fails with ErrConstraintUnsatisfiable, naming DV_QUANTITY.accuracy
// and the leaf's OPT path and no value, where it does not; and an accuracy
// that is no percentage, or a value with either attribute unset, is left
// alone.
func TestREQ107_SettleAccuracyLeavesWhatNoRuleBinds(t *testing.T) {
	const (
		trueOnly = `<true_valid>true</true_valid><false_valid>false</false_valid>`
		either   = `<true_valid>true</true_valid><false_valid>true</false_valid>`
		only150  = `<list>150</list>`
	)
	t.Run("narrowed", func(t *testing.T) {
		q := &rm.DVQuantity{Accuracy: new(rm.Real(0)), AccuracyIsPercent: new(true)}
		n := accuracyRoot(t, "", trueOnly)
		if err := settleAccuracy(n.root, q); err != nil || *q.Accuracy != 1 || !*q.AccuracyIsPercent {
			t.Errorf("settleAccuracy(0%%) = %v, accuracy %v percent %v, want nil, 1, true", err, *q.Accuracy, *q.AccuracyIsPercent)
		}
	})
	t.Run("percent made false", func(t *testing.T) {
		q := &rm.DVQuantity{Accuracy: new(rm.Real(150)), AccuracyIsPercent: new(true)}
		n := accuracyRoot(t, only150, either)
		if err := settleAccuracy(n.root, q); err != nil || *q.Accuracy != 150 || *q.AccuracyIsPercent {
			t.Errorf("settleAccuracy(150%%) = %v, accuracy %v percent %v, want nil, 150, false", err, *q.Accuracy, *q.AccuracyIsPercent)
		}
	})
	t.Run("unsatisfiable", func(t *testing.T) {
		q := &rm.DVQuantity{Accuracy: new(rm.Real(150)), AccuracyIsPercent: new(true)}
		n := accuracyRoot(t, only150, trueOnly)
		err := settleAccuracy(n.root, q)
		if !errors.Is(err, ErrConstraintUnsatisfiable) {
			t.Fatalf("settleAccuracy(150%%) error = %v, want ErrConstraintUnsatisfiable", err)
		}
		if msg := err.Error(); !strings.Contains(msg, "DV_QUANTITY.accuracy at "+n.path) || strings.Contains(msg, "150") {
			t.Errorf("settleAccuracy(150%%) error = %q, want it to name DV_QUANTITY.accuracy at %s and no value", msg, n.path)
		}
		if *q.Accuracy != 150 || !*q.AccuracyIsPercent {
			t.Errorf("settleAccuracy(150%%) changed the value to %v percent %v, want it kept", *q.Accuracy, *q.AccuracyIsPercent)
		}
	})
	n := accuracyRoot(t, only150, trueOnly)
	for _, q := range []*rm.DVQuantity{
		{Accuracy: new(rm.Real(150)), AccuracyIsPercent: new(false)},
		{Accuracy: new(rm.Real(150))},
		{AccuracyIsPercent: new(true)},
	} {
		before := *q
		if err := settleAccuracy(n.root, q); err != nil || q.Accuracy != before.Accuracy || q.AccuracyIsPercent != before.AccuracyIsPercent {
			t.Errorf("settleAccuracy(%+v) = %v and changed it to %+v, want nil and it kept", before, err, q)
		}
	}
}
