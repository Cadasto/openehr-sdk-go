package bmmgen

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestRenderWalkerPolymorphicNamesEachInterfacesClasses pins the interface
// table rendered into the JSON walker's own package, internal/rmjsonwalk
// (REQ-170, REQ-024): polymorphicClass has one case per hooked RM interface,
// naming the BMM class the interface stands for and, for a narrow interface
// only, the same class as its fallback, which is the class its decode hook
// falls back to. The table adds no public API: the rendered rm hooks file
// declares nothing beyond its init, and the AOM 1.4 target gets no table,
// since the JSON walk does not check AOM 1.4 classes.
func TestRenderWalkerPolymorphicNamesEachInterfacesClasses(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	hooksSrc, err := RenderJSONHooksFile(plan)
	if err != nil {
		t.Fatalf("RenderJSONHooksFile: %v", err)
	}
	table, err := RenderWalkerPolymorphicFile(plan)
	if err != nil {
		t.Fatalf("RenderWalkerPolymorphicFile: %v", err)
	}
	src := string(table)
	if !strings.Contains(src, "package rmjsonwalk\n") || !strings.Contains(src, "func polymorphicClass(p any) (class, fallback string, ok bool)") {
		t.Fatalf("rendered walker table is not the unexported polymorphicClass of package rmjsonwalk:\n%s", src)
	}

	hooks := regexp.MustCompile(`out \*(\w+)\) error \{\n\t\treturn typereg\.DecodePolymorphic\(dec, out, (nil|func\(\) any \{ return &(\w+)\{\} \})\)`).FindAllStringSubmatch(string(hooksSrc), -1)
	if len(hooks) < 10 {
		t.Fatalf("found %d decode hooks in the rendered rm hooks file; want the full set of RM interfaces", len(hooks))
	}
	cases := map[string][2]string{}
	for _, m := range regexp.MustCompile(`case \*rm\.(\w+):\n\t\treturn "(\w+)", "(\w*)", true`).FindAllStringSubmatch(src, -1) {
		cases[m[1]] = [2]string{m[2], m[3]}
	}
	if len(cases) != len(hooks) {
		t.Errorf("polymorphicClass has %d cases for %d hooked interfaces; want one per interface", len(cases), len(hooks))
	}
	for _, h := range hooks {
		iface, fallbackGo := h[1], h[3]
		c, ok := cases[iface]
		if !ok {
			t.Errorf("polymorphicClass has no case for the hooked interface %s", iface)
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
			t.Errorf("polymorphicClass(*rm.%s) = %q, want %q", iface, got, want)
		}
	}

	if decls := regexp.MustCompile(`(?m)^func [^i]`).FindAllString(string(hooksSrc), -1); len(decls) != 0 {
		t.Errorf("rendered rm hooks file declares %q beside init; the walker's table belongs in internal/rmjsonwalk", decls)
	}
	aom, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM 1.4): %v", err)
	}
	if got, err := RenderWalkerPolymorphicFile(aom); err != nil || got != nil {
		t.Errorf("RenderWalkerPolymorphicFile(AOM 1.4) = %d bytes, %v, want nothing: the JSON walk does not check AOM 1.4", len(got), err)
	}
}
