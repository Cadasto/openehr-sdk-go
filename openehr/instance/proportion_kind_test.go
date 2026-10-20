package instance_test

import (
	"fmt"
	"math"
	mrand "math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// corpusSeeds are the PCG seeds the corpus checks run RandomFill with, on
// the stream corpusStream. Seeds 2 and 3 are the ones that gave
// Demonstration.v1 a percent with denominator 1.
var corpusSeeds = []uint64{1, 2, 3, 4}

const corpusStream = 20261020

// corpusRun is one root Generate returned for a vendored template.
type corpusRun struct {
	name     string
	raw      []byte
	compiled *templatecompile.Compiled
	opts     instance.Options
	seed     uint64
	out      any
}

func (r corpusRun) String() string {
	return fmt.Sprintf("%s/%v/%v/seed=%d", r.name, r.opts.Policy, r.opts.ValueFill, r.seed)
}

// corpusSetting is one setting a corpus check generates a template under.
// The seed is the RandomFill one; each call gets a fresh source of it.
type corpusSetting struct {
	opts instance.Options
	seed uint64
}

// corpusSettings are the settings each corpus check generates a template
// under: every policy, ExampleFill once, and RandomFill once per seed of
// corpusSeeds.
func corpusSettings(composition bool) []corpusSetting {
	var out []corpusSetting
	for _, policy := range everyPolicy {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			seeds := corpusSeeds
			if fill == instance.ExampleFill {
				seeds = seeds[:1]
			}
			for _, seed := range seeds {
				opts := instance.Options{
					Policy:    policy,
					ValueFill: fill,
					Language:  "en",
					Now:       time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
				}
				if composition {
					opts.Territory, opts.Composer = "NL", testComposer()
				}
				out = append(out, corpusSetting{opts: opts, seed: seed})
			}
		}
	}
	return out
}

// eachCorpusRun generates every vendored template that parses and
// compiles under corpusSettings and calls check with each root Generate
// returns. A template Generate refuses is skipped: its refusal is the
// corpus ratchet's business. It fails t when no template was generated.
func eachCorpusRun(t *testing.T, check func(run corpusRun)) {
	t.Helper()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	runs := 0
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			continue
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", ref.Path, err)
		}
		composition := c.Root() != nil && c.Root().RMTypeName() == "COMPOSITION"
		for _, setting := range corpusSettings(composition) {
			opts := setting.opts
			opts.ValueSource = mrand.NewPCG(setting.seed, corpusStream)
			out, err := instance.Generate(t.Context(), c, opts)
			if err != nil {
				continue
			}
			runs++
			check(corpusRun{name: ref.Name, raw: raw, compiled: c, opts: opts, seed: setting.seed, out: out})
		}
	}
	if runs == 0 {
		t.Fatal("no vendored template generated, so nothing was checked")
	}
}

// visitRM calls visit with every value of type T in the RM tree under
// root, held by value or by pointer, with a slash-separated path of the
// canonical JSON attribute names to it. A pointer reached twice is walked
// once.
func visitRM[T any](root any, visit func(path string, v T)) {
	visitRMIn(root, func(path, _ string, v T) { visit(path, v) })
}

// visitRMIn is visitRM that also passes the archetype id of the nearest
// enclosing archetype root of each value: the nearest object at or above
// it that carries archetype_details, or "" when there is none.
func visitRMIn[T any](root any, visit func(path, archetype string, v T)) {
	target := reflect.TypeFor[T]()
	details := reflect.TypeFor[*rm.Archetyped]()
	seen := make(map[uintptr]bool)
	var walk func(rv reflect.Value, path, archetype string)
	walk = func(rv reflect.Value, path, archetype string) {
		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() || seen[rv.Pointer()] {
				return
			}
			seen[rv.Pointer()] = true
			walk(rv.Elem(), path, archetype)
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem(), path, archetype)
			}
		case reflect.Struct:
			if f := rv.FieldByName("ArchetypeDetails"); f.IsValid() && f.Type() == details && !f.IsNil() {
				archetype = f.Interface().(*rm.Archetyped).ArchetypeID.Value
			}
			if rv.Type() == target {
				visit(path, archetype, rv.Interface().(T))
			}
			for i := range rv.NumField() {
				field := rv.Type().Field(i)
				if !field.IsExported() {
					continue
				}
				walk(rv.Field(i), path+"/"+jsonName(field), archetype)
			}
		case reflect.Slice, reflect.Array:
			for i := range rv.Len() {
				walk(rv.Index(i), fmt.Sprintf("%s[%d]", path, i), archetype)
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Value(), fmt.Sprintf("%s[%v]", path, iter.Key()), archetype)
			}
		}
	}
	walk(reflect.ValueOf(root), "", "")
}

// jsonName is the canonical JSON name of an RM struct field, or the Go
// name when it has none.
func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" || name == "-" {
		return f.Name
	}
	return name
}

// proportionKindBreaks names the DV_PROPORTION invariants p breaks among
// those its type decides (pinned RM BMM, class DV_PROPORTION; codes from
// PROPORTION_KIND: 0 ratio, 1 unitary, 2 percent, 3 fraction, 4 integer
// fraction), plus Valid_denominator and Precision_validity. A whole number
// is read as the floor's Precision_validity reads it.
func proportionKindBreaks(p rm.DVProportion) []string {
	whole := func(v rm.Real) bool {
		f := float64(v)
		return !math.IsInf(f, 0) && math.Trunc(f) == f
	}
	var out []string
	if p.Type < 0 || p.Type > 4 {
		out = append(out, "Type_validity")
	}
	if p.Type == 1 && p.Denominator != 1 {
		out = append(out, "Unitary_validity")
	}
	if p.Type == 2 && p.Denominator != 100 {
		out = append(out, "Percent_validity")
	}
	if (p.Type == 3 || p.Type == 4) && !(whole(p.Numerator) && whole(p.Denominator)) {
		out = append(out, "Fraction_validity")
	}
	if p.Denominator == 0 {
		out = append(out, "Valid_denominator")
	}
	if p.Precision != nil && *p.Precision == 0 && !(whole(p.Numerator) && whole(p.Denominator)) {
		out = append(out, "Precision_validity")
	}
	return out
}

func proportionText(p rm.DVProportion) string {
	precision := "none"
	if p.Precision != nil {
		precision = fmt.Sprint(*p.Precision)
	}
	return fmt.Sprintf("%v/%v type %d precision %s", p.Numerator, p.Denominator, p.Type, precision)
}

// proportionsOf returns every DV_PROPORTION in root, in walk order.
func proportionsOf(root any) []rm.DVProportion {
	var out []rm.DVProportion
	visitRM(root, func(_ string, p rm.DVProportion) { out = append(out, p) })
	return out
}

// TestREQ107_CorpusProportionsMeetTheirKindRules is the REQ-107 Output
// check, on the RM invariants the floor does not evaluate yet, that every
// DV_PROPORTION the generator writes for a vendored template meets the
// rules of its kind (Type_validity, Unitary_validity, Percent_validity,
// Fraction_validity, Valid_denominator) and Precision_validity, under
// every policy, both value fills and several fixed RandomFill seeds. It
// also checks that a second call with the same seed writes the same
// proportions (REQ-107 Primitive-leaf value fill).
func TestREQ107_CorpusProportionsMeetTheirKindRules(t *testing.T) {
	kinds := make(map[rm.Integer]int)
	eachCorpusRun(t, func(run corpusRun) {
		visitRM(run.out, func(path string, p rm.DVProportion) {
			kinds[p.Type]++
			if breaks := proportionKindBreaks(p); len(breaks) > 0 {
				t.Errorf("%s: DV_PROPORTION %s at %s breaks %v", run, proportionText(p), path, breaks)
			}
		})
		if run.opts.ValueFill != instance.RandomFill {
			return
		}
		again := run.opts
		again.ValueSource = mrand.NewPCG(run.seed, corpusStream)
		out, err := instance.Generate(t.Context(), run.compiled, again)
		if err != nil {
			t.Fatalf("%s: second Generate: %v", run, err)
		}
		if first, second := proportionsOf(run.out), proportionsOf(out); !reflect.DeepEqual(first, second) {
			t.Errorf("%s: proportions differ between two calls with one seed: %v and %v", run, first, second)
		}
	})
	// The corpus holds a percent kind and a fraction kind; a check that
	// never met them proves nothing about their rules.
	for _, kind := range []rm.Integer{2, 3} {
		if kinds[kind] == 0 {
			t.Errorf("no DV_PROPORTION of type %d generated, so its rule was not checked (types seen: %v)", kind, kinds)
		}
	}
}

// optProportion is an ELEMENT template whose value is a DV_PROPORTION. Each
// body is the item of the C_REAL on numerator or denominator, or of the
// C_INTEGER on type or precision; "" leaves that attribute out.
func optProportion(numerator, denominator, kind, precision string) string {
	var attrs []string
	for _, a := range []struct{ name, rmType, item, body string }{
		{"numerator", "REAL", "C_REAL", numerator},
		{"denominator", "REAL", "C_REAL", denominator},
		{"type", "INTEGER", "C_INTEGER", kind},
		{"precision", "INTEGER", "C_INTEGER", precision},
	} {
		if a.body != "" {
			attrs = append(attrs, optSingle(a.name, optPrimitive(a.rmType, a.item, a.body)))
		}
	}
	return optTemplate("ELEMENT", optSingle("value", optNode("DV_PROPORTION", "", attrs...)))
}

// proportionSeeds is how many RandomFill seeds each synthetic proportion
// case runs.
const proportionSeeds = 20

// eachProportionRun generates c under every policy, ExampleFill once and
// RandomFill once per seed below proportionSeeds, and calls check with the
// one DV_PROPORTION each root holds.
func eachProportionRun(t *testing.T, c *templatecompile.Compiled, check func(t *testing.T, what string, out any, p rm.DVProportion)) {
	t.Helper()
	for _, policy := range everyPolicy {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			seeds := uint64(proportionSeeds)
			if fill == instance.ExampleFill {
				seeds = 1
			}
			for seed := range seeds {
				opts := instance.Options{Policy: policy, ValueFill: fill, ValueSource: mrand.NewPCG(seed, corpusStream), Now: defaultsNow}
				what := fmt.Sprintf("%v/%v/seed=%d", policy, fill, seed)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: Generate: %v", what, err)
				}
				ps := proportionsOf(out)
				if len(ps) != 1 {
					t.Fatalf("%s: %d DV_PROPORTION generated, want 1", what, len(ps))
				}
				check(t, what, out, ps[0])
			}
		}
	}
}

// TestREQ107_ProportionSettlesToItsKind is the REQ-107 check that the
// generator settles a DV_PROPORTION so it meets the rules of its kind
// inside the OPT's own constraints on numerator, denominator, type and
// precision (Output; Precedence 1 and 3): a percent takes the denominator
// 100, a fraction and a value of precision 0 whole numbers, a ratio a
// denominator that is not 0, and a type the OPT allows outside the
// proportion kinds, or one whose rule its constraints cannot meet, gives
// way to a kind they can. The RM floor and the template validator pass.
func TestREQ107_ProportionSettlesToItsKind(t *testing.T) {
	r, n := instance.RealRange, instance.IntRange
	closed, open, noEnd := instance.Closed, instance.Open, instance.NoEnd
	cases := []struct {
		name string
		opt  string
		// want is a check of the settled value beyond the kind rules.
		want func(p rm.DVProportion) bool
	}{
		{
			name: "percent with an open denominator",
			opt:  optProportion("", "", instance.IntList(2), ""),
			want: func(p rm.DVProportion) bool { return p.Type == 2 && p.Denominator == 100 },
		},
		{
			name: "fraction over real ranges",
			opt:  optProportion(r(closed(0), closed(10)), r(open(0), closed(10)), instance.IntList(3), ""),
			want: func(p rm.DVProportion) bool { return p.Type == 3 },
		},
		{
			name: "integer fraction over a real list",
			opt:  optProportion(instance.RealList(0.5, 2), "", instance.IntList(4), ""),
			want: func(p rm.DVProportion) bool { return p.Type == 4 && p.Numerator == 2 },
		},
		{
			name: "ratio of precision 0 over a real range",
			opt:  optProportion(r(closed(0), closed(10)), "", instance.IntList(0), instance.IntList(0)),
			want: func(p rm.DVProportion) bool { return p.Type == 0 && p.Precision != nil && *p.Precision == 0 },
		},
		{
			name: "ratio whose denominator range starts at 0",
			opt:  optProportion("", r(closed(0), noEnd), instance.IntList(0), ""),
			want: func(p rm.DVProportion) bool { return p.Type == 0 && p.Denominator > 0 },
		},
		{
			name: "percent the denominator range rules out gives way to a ratio",
			opt:  optProportion("", r(closed(1), closed(50)), instance.IntList(2, 0), ""),
			want: func(p rm.DVProportion) bool { return p.Type == 0 },
		},
		{
			name: "type range beyond the proportion kinds",
			opt:  optProportion("", "", n(closed(0), closed(9)), ""),
			want: func(p rm.DVProportion) bool { return p.Type >= 0 && p.Type <= 4 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			eachProportionRun(t, c, func(t *testing.T, what string, out any, p rm.DVProportion) {
				if breaks := proportionKindBreaks(p); len(breaks) > 0 {
					t.Errorf("%s: DV_PROPORTION %s breaks %v", what, proportionText(p), breaks)
				}
				if !tc.want(p) {
					t.Errorf("%s: DV_PROPORTION %s is not the settled value this case wants", what, proportionText(p))
				}
				checkValid(t, out, c)
			})
		})
	}
}

// TestREQ107_ProportionKeepsTheOPTWhereNoKindFits is the REQ-107
// Precedence 1 check that the OPT wins where its constraints admit no
// value the rule of any kind it allows accepts: a percent whose
// denominator the OPT limits to 1..50 keeps the type and the denominator
// the walk wrote, which the template layer accepts, and breaks
// Percent_validity (§ Exceptions and known gaps), which the RM floor
// reports (REQ-112) as the one error of the run.
func TestREQ107_ProportionKeepsTheOPTWhereNoKindFits(t *testing.T) {
	c := compileOPTText(t, optProportion("", instance.RealRange(instance.Closed(1), instance.Closed(50)), instance.IntList(2), ""), true)
	eachProportionRun(t, c, func(t *testing.T, what string, out any, p rm.DVProportion) {
		if p.Type != 2 || p.Denominator < 1 || p.Denominator > 50 {
			t.Errorf("%s: DV_PROPORTION %s, want the walk's type 2 over a denominator in 1..50", what, proportionText(p))
		}
		issues := templateErrors(out, c)
		if len(issues) != 1 || issues[0].Layer != validation.LayerRM || issues[0].Rule != "DV_PROPORTION.Percent_validity" {
			t.Errorf("%s: validator errors %v, want the one RM-layer DV_PROPORTION.Percent_validity", what, issues)
		}
	})
}

// TestREQ107_ProportionIntervalBoundsKeepTheirKind is the REQ-107 check
// (Output, Intervals) that the bounds of a DV_INTERVAL<DV_PROPORTION>, and
// of a DV_INTERVAL<DV_ORDERED> holding proportions, are settled to their
// kind before they are put in order, and stay so once they are, inside
// their own side's OPT constraints: the in-order pair the generator writes
// keeps whole numbers wherever a bound is a fraction or has precision 0,
// whether the two sides constrain precision differently or draw it each
// on its own from one range, and the RM floor and the template validator
// pass. Each case's two numerator ranges overlap, so RandomFill draws the
// bounds either way round, and each case admits an in-order pair, which
// the generator writes even where it needs a bound to take a precision
// other than the one it drew, or, where its numerator range has no end that
// way, the other bound's ratio on its own denominator.
func TestREQ107_ProportionIntervalBoundsKeepTheirKind(t *testing.T) {
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	r, closed, noEnd := instance.RealRange, instance.Closed, instance.NoEnd
	tenth := r(closed(0), closed(10))
	ratio := instance.IntList(0)
	upToTwo := instance.IntRange(closed(0), closed(2))
	zero, two := instance.RequiredPrecision(instance.IntList(0)), instance.RequiredPrecision(instance.IntList(2))
	// Only numerators from 0.5 to 0.7 put these two sides in order, so a
	// lower bound that drew precision 0, whose numerator is then a whole
	// number of at least 1, is in order only with another precision.
	belowOne, upToPointSeven := r(closed(0.5), closed(10)), r(closed(0), closed(0.7))
	cases := []struct {
		name         string
		lower, upper string
	}{
		{
			name:  "fractions over numerator ranges that overlap in part",
			lower: instance.ProportionKindBound(r(closed(5.5), closed(10.5)), instance.IntList(3), ""),
			upper: instance.ProportionKindBound(r(closed(0), closed(7.5)), instance.IntList(3), ""),
		},
		{
			name:  "ratios of precision 0 below and no precision above",
			lower: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(instance.IntList(0))),
			upper: instance.ProportionKindBound(tenth, ratio, ""),
		},
		{
			name:  "ratios of no precision below and precision 0 above",
			lower: instance.ProportionKindBound(tenth, ratio, ""),
			upper: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(instance.IntList(0))),
		},
		{
			name:  "ratios that both draw a precision in 0..2",
			lower: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(upToTwo)),
			upper: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(upToTwo)),
		},
		{
			name:  "ratios of precision 2 below and precision 0 above",
			lower: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(instance.IntList(2))),
			upper: instance.ProportionKindBound(tenth, ratio, instance.RequiredPrecision(instance.IntList(0))),
		},
		{
			name:  "ratios of an optional precision below and a prohibited one above",
			lower: instance.ProportionKindBound(tenth, ratio, instance.OptionalPrecision(upToTwo)),
			upper: instance.ProportionKindBound(tenth, ratio, instance.ProhibitedPrecision()),
		},
		{
			name:  "ratios of precision 0 below and precision 2 above a numerator open upward",
			lower: instance.ProportionKindBound(tenth, ratio, zero),
			upper: instance.ProportionKindBound(r(closed(0), noEnd), ratio, two),
		},
		{
			name:  "ratios of precision 0 below and a prohibited precision above a numerator open upward",
			lower: instance.ProportionKindBound(tenth, ratio, zero),
			upper: instance.ProportionKindBound(r(closed(0), noEnd), ratio, instance.ProhibitedPrecision()),
		},
		{
			name:  "ratios of precision 2 below a numerator open downward and precision 0 above",
			lower: instance.ProportionKindBound(r(noEnd, closed(10)), ratio, two),
			upper: instance.ProportionKindBound(r(closed(-90), closed(-80)), ratio, zero),
		},
		{
			// The lower bound takes the upper one's ratio rounded down to a
			// whole number.
			name:  "ratios of precision 0 below a numerator open downward and precision 2 above",
			lower: instance.ProportionKindBound(r(noEnd, closed(10)), ratio, zero),
			upper: instance.ProportionKindBound(r(closed(-90), closed(-80)), ratio, two),
		},
		{
			// The upper bound takes the lower one's ratio rounded up to a
			// whole number.
			name:  "ratios of precision 2 below and precision 0 above a numerator open upward",
			lower: instance.ProportionKindBound(r(closed(90), closed(100)), ratio, two),
			upper: instance.ProportionKindBound(r(closed(0), noEnd), ratio, zero),
		},
		{
			name:  "ratios in order only below 1, of an optional precision 0 or 2 below and an optional precision 2 above",
			lower: instance.ProportionKindBound(belowOne, ratio, instance.OptionalPrecision(instance.IntList(0, 2))),
			upper: instance.ProportionKindBound(upToPointSeven, ratio, instance.OptionalPrecision(instance.IntList(2))),
		},
		{
			name:  "ratios in order only below 1, of a precision 0 or 2 below and precision 2 above",
			lower: instance.ProportionKindBound(belowOne, ratio, instance.RequiredPrecision(instance.IntList(0, 2))),
			upper: instance.ProportionKindBound(upToPointSeven, ratio, two),
		},
		{
			name:  "ratios in order only below 1, of a precision in 0..2 below and precision 2 above",
			lower: instance.ProportionKindBound(belowOne, ratio, instance.RequiredPrecision(upToTwo)),
			upper: instance.ProportionKindBound(upToPointSeven, ratio, two),
		},
		{
			name:  "ratios in order only below 1, of a precision in -1..0 below and precision 2 above",
			lower: instance.ProportionKindBound(belowOne, ratio, instance.RequiredPrecision(instance.IntRange(closed(-1), closed(0)))),
			upper: instance.ProportionKindBound(upToPointSeven, ratio, two),
		},
		{
			// The lowest whole numerator the lower side lists is 3, not 0.5
			// rounded up.
			name:  "ratios of precision 0 over a numerator list of 0.5, 3 and 8 below and precision 2 above",
			lower: instance.ProportionKindBound(instance.RealList(0.5, 3, 8), ratio, zero),
			upper: instance.ProportionKindBound(r(closed(0), closed(5)), ratio, two),
		},
		{
			// The lower bound takes -86/3 on its own denominator 7, where
			// -86*7/3 divided by 7 lands a float64 step above -86/3.
			name:  "ratios over a denominator of 7 below a numerator open downward and of 3 above",
			lower: instance.ProportionKindBoundOver(r(noEnd, closed(-80)), instance.RealList(7), ratio, ""),
			upper: instance.ProportionKindBoundOver(instance.RealList(-86), instance.RealList(3), ratio, ""),
		},
		{
			// The upper bound takes 86/3 on its own denominator 7, where
			// 86*7/3 divided by 7 lands a float64 step below 86/3.
			name:  "ratios over a denominator of 3 below and of 7 above a numerator open upward",
			lower: instance.ProportionKindBoundOver(instance.RealList(86), instance.RealList(3), ratio, ""),
			upper: instance.ProportionKindBoundOver(r(closed(0), noEnd), instance.RealList(7), ratio, ""),
		},
		{
			// The upper bound's denominator 2.5 is not a whole number, so the
			// lower bound of precision 0 cannot take it.
			name:  "ratios of precision 0 below a numerator open downward and precision 2 over a denominator of 2.5 above",
			lower: instance.ProportionKindBound(r(noEnd, closed(10)), ratio, zero),
			upper: instance.ProportionKindBoundOver(r(closed(-90), closed(-80)), instance.RealList(2.5), ratio, two),
		},
		{
			// A negative denominator makes a higher numerator the lower ratio.
			name:  "ratios over a denominator of -1, of precision 0 below a numerator open upward and precision 2 above",
			lower: instance.ProportionKindBoundOver(r(closed(0), noEnd), instance.RealList(-1), ratio, zero),
			upper: instance.ProportionKindBoundOver(r(closed(90), closed(100)), instance.RealList(-1), ratio, two),
		},
	}
	for _, tc := range cases {
		for _, param := range []string{"DV_PROPORTION", "DV_ORDERED"} {
			t.Run(tc.name+"/"+param, func(t *testing.T) {
				c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, param, tc.lower, tc.upper))
				compared := 0
				for seed := range uint64(orderSeeds) {
					opts := intervalOptions(instance.Large)
					opts.ValueFill = instance.RandomFill
					opts.ValueSource = mrand.NewPCG(seed, 1)
					root, doc := generateWithJSON(t, c, opts)
					visitRM(root, func(path string, p rm.DVProportion) {
						if breaks := proportionKindBreaks(p); len(breaks) > 0 {
							t.Errorf("seed %d: DV_PROPORTION %s at %s breaks %v", seed, proportionText(p), path, breaks)
						}
					})
					walkIntervals(doc, "", func(path string, iv map[string]any) {
						if _, _, ok := boundedPair(iv); !ok {
							return
						}
						compared++
						proportionInOrder(t, seed, path, iv)
					})
					checkValid(t, root, c)
				}
				if compared == 0 {
					t.Fatal("no bounded DV_INTERVAL generated, so nothing was checked")
				}
			})
		}
	}
}

// eachProportionIntervalRun compiles the vendored DV_COUNT interval OPT
// with lower and upper as its two bound nodes, as a DV_INTERVAL<DV_PROPORTION>
// and as a DV_INTERVAL<DV_ORDERED>, and calls check, once for each, with the
// canonical JSON of each root Generate returns under fill: one for
// ExampleFill, orderSeeds for RandomFill, one per seed.
func eachProportionIntervalRun(t *testing.T, lower, upper string, fill instance.ValueFill, check func(t *testing.T, docs []any)) {
	t.Helper()
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	for _, param := range []string{"DV_PROPORTION", "DV_ORDERED"} {
		t.Run(param, func(t *testing.T) {
			c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, param, lower, upper))
			seeds := uint64(orderSeeds)
			if fill == instance.ExampleFill {
				seeds = 1
			}
			var docs []any
			for seed := range seeds {
				opts := intervalOptions(instance.Large)
				opts.ValueFill = fill
				opts.ValueSource = mrand.NewPCG(seed, 1)
				_, doc := generateWithJSON(t, c, opts)
				docs = append(docs, doc)
			}
			check(t, docs)
		})
	}
}

// TestREQ107_ProportionIntervalAdoptionKeepsThePrecisionItChooses is the
// REQ-107 check (Output, Intervals) that a bound which takes the other
// bound's ratio, because its numerator range has no low end, takes the
// precision its side admits and that needs no whole numbers, not the one
// the walk drew: the lower side admits an optional precision 0 or 2 and
// the upper side the numerators -90 to -80, so where the pair is put in
// order (the upper bound then sits at -80) the lower bound takes the upper
// one's numerator, a real number, with no precision 0 and no rounding to a
// whole number, even where the walk drew precision 0.
func TestREQ107_ProportionIntervalAdoptionKeepsThePrecisionItChooses(t *testing.T) {
	r, closed, noEnd := instance.RealRange, instance.Closed, instance.NoEnd
	ratio := instance.IntList(0)
	lower := instance.ProportionKindBound(r(noEnd, closed(10)), ratio, instance.OptionalPrecision(instance.IntList(0, 2)))
	upper := instance.ProportionKindBound(r(closed(-90), closed(-80)), ratio, instance.RequiredPrecision(instance.IntList(2)))
	eachProportionIntervalRun(t, lower, upper, instance.RandomFill, func(t *testing.T, docs []any) {
		ordered := 0
		for seed, doc := range docs {
			walkIntervals(doc, "", func(path string, iv map[string]any) {
				lo, hi, ok := boundedPair(iv)
				if !ok {
					return
				}
				// A pair the walk drew in order is left alone, and its lower bound
				// may keep any precision its side admits.
				if top, _ := hi["numerator"].(float64); top != -80 {
					return
				}
				ordered++
				numerator, _ := lo["numerator"].(float64)
				if precision, has := lo["precision"].(float64); has && precision == 0 {
					t.Errorf("seed %d, %s: lower bound %v/%v has precision 0, but its side admits one that needs no whole number", seed, path, lo["numerator"], lo["denominator"])
				}
				if numerator == math.Trunc(numerator) {
					t.Errorf("seed %d, %s: lower bound numerator %v is a whole number, want the upper bound's real one", seed, path, numerator)
				}
			})
		}
		if ordered == 0 {
			t.Fatal("no pair was put in order, so nothing was checked")
		}
	})
}

// TestREQ107_ProportionIntervalLeavesAPairWhoseAdoptionOverflows is the
// REQ-107 check (Output, Intervals) that a pair no finite replacement puts
// in order is left as generated: the lower bound has no low end, so it
// would take the upper bound's ratio over its own denominator 1e10, a
// numerator of -1e310 that float64 cannot hold. The pair stays as the walk
// wrote it, which canjson.Marshal accepts, with the upper bound's -1e300
// over 1 and a finite lower numerator over 1e10.
func TestREQ107_ProportionIntervalLeavesAPairWhoseAdoptionOverflows(t *testing.T) {
	r, closed, noEnd := instance.RealRange, instance.Closed, instance.NoEnd
	ratio := instance.IntList(0)
	lower := instance.ProportionKindBoundOver(r(noEnd, closed(0)), instance.RealList(1e10), ratio, "")
	upper := instance.ProportionKindBoundOver(instance.RealList(-1e300), instance.RealList(1), ratio, "")
	eachProportionIntervalRun(t, lower, upper, instance.RandomFill, func(t *testing.T, docs []any) {
		compared := 0
		for seed, doc := range docs {
			walkIntervals(doc, "", func(path string, iv map[string]any) {
				lo, hi, ok := boundedPair(iv)
				if !ok {
					return
				}
				compared++
				if n, _ := lo["numerator"].(float64); math.IsInf(n, 0) || math.IsNaN(n) || lo["denominator"] != 1e10 {
					t.Errorf("seed %d, %s: lower bound %v/%v, want a finite numerator over 1e10 as generated", seed, path, lo["numerator"], lo["denominator"])
				}
				if hi["numerator"] != -1e300 || hi["denominator"] != 1.0 {
					t.Errorf("seed %d, %s: upper bound %v/%v, want -1e300/1 as generated", seed, path, hi["numerator"], hi["denominator"])
				}
			})
		}
		if compared == 0 {
			t.Fatal("no bounded DV_INTERVAL generated, so nothing was checked")
		}
	})
}

// TestREQ107_ProportionIntervalOrderingWritesNoPrecisionBelowTheFloor is
// the REQ-107 check (Output, Intervals) that putting a pair in order never
// gives a bound a precision the RM floor rejects (DV_PROPORTION precision
// at least -1, REQ-112) where the walk did not draw it. The lower side
// lists the precisions 0 and -2 and the numerators 0.5 to 10, the upper
// side the numerators 0 to 0.7, so the lower bound is in order only up to
// 0.7, which precision 0 cannot hold: the one precision that would put the
// pair in order is -2, and the pair stays as generated. ExampleFill draws
// the first precision the lower side lists, 0, so a -2 in the output is the
// ordering's.
func TestREQ107_ProportionIntervalOrderingWritesNoPrecisionBelowTheFloor(t *testing.T) {
	r, closed := instance.RealRange, instance.Closed
	ratio := instance.IntList(0)
	lower := instance.ProportionKindBound(r(closed(0.5), closed(10)), ratio, instance.RequiredPrecision(instance.IntList(0, -2)))
	upper := instance.ProportionKindBound(r(closed(0), closed(0.7)), ratio, instance.RequiredPrecision(instance.IntList(2)))
	eachProportionIntervalRun(t, lower, upper, instance.ExampleFill, func(t *testing.T, docs []any) {
		compared := 0
		for seed, doc := range docs {
			walkIntervals(doc, "", func(path string, iv map[string]any) {
				lo, hi, ok := boundedPair(iv)
				if !ok {
					return
				}
				compared++
				for _, bound := range []map[string]any{lo, hi} {
					if precision, has := bound["precision"].(float64); has && precision < -1 {
						t.Errorf("seed %d, %s: bound %v/%v has precision %v, below the RM floor's -1", seed, path, bound["numerator"], bound["denominator"], precision)
					}
				}
			})
		}
		if compared == 0 {
			t.Fatal("no bounded DV_INTERVAL generated, so nothing was checked")
		}
	})
}
