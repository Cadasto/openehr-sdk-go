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
	target := reflect.TypeFor[T]()
	seen := make(map[uintptr]bool)
	var walk func(rv reflect.Value, path string)
	walk = func(rv reflect.Value, path string) {
		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() || seen[rv.Pointer()] {
				return
			}
			seen[rv.Pointer()] = true
			walk(rv.Elem(), path)
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem(), path)
			}
		case reflect.Struct:
			if rv.Type() == target {
				visit(path, rv.Interface().(T))
			}
			for i := range rv.NumField() {
				field := rv.Type().Field(i)
				if !field.IsExported() {
					continue
				}
				walk(rv.Field(i), path+"/"+jsonName(field))
			}
		case reflect.Slice, reflect.Array:
			for i := range rv.Len() {
				walk(rv.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		case reflect.Map:
			iter := rv.MapRange()
			for iter.Next() {
				walk(iter.Value(), fmt.Sprintf("%s[%v]", path, iter.Key()))
			}
		}
	}
	walk(reflect.ValueOf(root), "")
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
// the walk wrote, which the template validator accepts, and breaks
// Percent_validity (§ Exceptions and known gaps).
func TestREQ107_ProportionKeepsTheOPTWhereNoKindFits(t *testing.T) {
	c := compileOPTText(t, optProportion("", instance.RealRange(instance.Closed(1), instance.Closed(50)), instance.IntList(2), ""), true)
	eachProportionRun(t, c, func(t *testing.T, what string, out any, p rm.DVProportion) {
		if p.Type != 2 || p.Denominator < 1 || p.Denominator > 50 {
			t.Errorf("%s: DV_PROPORTION %s, want the walk's type 2 over a denominator in 1..50", what, proportionText(p))
		}
		if issues := templateErrors(out, c); len(issues) > 0 {
			t.Errorf("%s: template validator: %v", what, issues)
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
// bounds either way round.
func TestREQ107_ProportionIntervalBoundsKeepTheirKind(t *testing.T) {
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	r, closed := instance.RealRange, instance.Closed
	tenth := r(closed(0), closed(10))
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
			lower: instance.ProportionKindBound(tenth, instance.IntList(0), instance.IntList(0)),
			upper: instance.ProportionKindBound(tenth, instance.IntList(0), ""),
		},
		{
			name:  "ratios of no precision below and precision 0 above",
			lower: instance.ProportionKindBound(tenth, instance.IntList(0), ""),
			upper: instance.ProportionKindBound(tenth, instance.IntList(0), instance.IntList(0)),
		},
		{
			name:  "ratios that both draw a precision in 0..2",
			lower: instance.ProportionKindBound(tenth, instance.IntList(0), instance.IntRange(closed(0), closed(2))),
			upper: instance.ProportionKindBound(tenth, instance.IntList(0), instance.IntRange(closed(0), closed(2))),
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
