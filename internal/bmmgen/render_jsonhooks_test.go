package bmmgen

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestRenderJSONHooksNamesEachInterfacesClasses pins the type information
// rendered beside the decode hooks (REQ-170, REQ-024): PolymorphicClass has
// one case per hooked interface, naming the BMM class the interface stands
// for and, for a narrow interface only, the same class as its fallback, which
// is the class its hook falls back to; and each concrete generic class with
// one type parameter gets a BMMTypeArgument method returning a new zero value
// of its type argument. A reader of a decode target's Go type learns these
// classes from methods instead of reflecting over the registry.
func TestRenderJSONHooksNamesEachInterfacesClasses(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	got, err := RenderJSONHooksFile(plan)
	if err != nil {
		t.Fatalf("RenderJSONHooksFile: %v", err)
	}
	src := string(got)

	hooks := regexp.MustCompile(`out \*(\w+)\) error \{\n\t\treturn typereg\.DecodePolymorphic\(dec, out, (nil|func\(\) any \{ return &(\w+)\{\} \})\)`).FindAllStringSubmatch(src, -1)
	if len(hooks) < 10 {
		t.Fatalf("found %d decode hooks in the rendered file; want the full set of RM interfaces", len(hooks))
	}
	cases := map[string][2]string{}
	for _, m := range regexp.MustCompile(`case \*(\w+):\n\t\treturn "(\w+)", "(\w*)", true`).FindAllStringSubmatch(src, -1) {
		cases[m[1]] = [2]string{m[2], m[3]}
	}
	if len(cases) != len(hooks) {
		t.Errorf("PolymorphicClass has %d cases for %d hooked interfaces; want one per interface", len(cases), len(hooks))
	}
	for _, h := range hooks {
		iface, fallbackGo := h[1], h[3]
		c, ok := cases[iface]
		if !ok {
			t.Errorf("PolymorphicClass has no case for the hooked interface %s", iface)
			continue
		}
		class, fallback := c[0], c[1]
		switch {
		case fallbackGo == "" && fallback != "":
			t.Errorf("%s: fallback %q, want none: its hook refuses an elided _type", iface, fallback)
		case fallbackGo != "" && fallback != class:
			t.Errorf("%s: class %q, fallback %q, want the parent class as both: its hook falls back to %s", iface, class, fallback, fallbackGo)
		case fallbackGo != "" && plan.Classes[class].GoName != fallbackGo:
			t.Errorf("%s: fallback %q is Go type %s, want %s, the type its hook falls back to", iface, fallback, plan.Classes[class].GoName, fallbackGo)
		}
	}
	for iface, want := range map[string][2]string{
		"DataValue":  {"DATA_VALUE", ""},
		"DVOrdered":  {"DV_ORDERED", ""},
		"DVTextLike": {"DV_TEXT", "DV_TEXT"},
	} {
		if got := cases[iface]; got != want {
			t.Errorf("PolymorphicClass(*%s) = %q, want %q", iface, got, want)
		}
	}

	for _, want := range []string{
		"func (*DVInterval[T]) BMMTypeArgument() any { return new(T) }",
		"func (*OriginalVersion[T]) BMMTypeArgument() any { return new(T) }",
		"func (*PointInterval[T]) BMMTypeArgument() any { return new(T) }",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("rendered hooks file lacks %q", want)
		}
	}
	if strings.Contains(src, "func (*Interval[T]) BMMTypeArgument()") {
		t.Error("rendered hooks file gives the abstract BASE Interval a BMMTypeArgument method; only concrete classes are decode targets")
	}
}
