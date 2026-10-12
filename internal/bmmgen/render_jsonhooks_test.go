package bmmgen

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestRenderJSONHooksRegistersTheWalkersTable pins the interface table the RM
// hooks file registers for the canonical-JSON walker (REQ-170, REQ-024,
// module-layout: canjson does not import openehr/rm). polymorphicClass is
// unexported, so the table adds no public API; it has one case per hooked
// interface, naming the BMM class the interface stands for, which for a
// narrow interface is the parent class its hook falls back to and for any
// other is an abstract class; and init registers it into
// internal/rmjsonwalk/polyreg. The AOM 1.4 hooks file gets no table, since
// the JSON walk does not check AOM 1.4 classes.
func TestRenderJSONHooksRegistersTheWalkersTable(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	got, err := RenderJSONHooksFile(plan)
	if err != nil {
		t.Fatalf("RenderJSONHooksFile: %v", err)
	}
	src := string(got)
	for _, want := range []string{
		"\t" + `"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk/polyreg"`,
		"\tpolyreg.Register(polymorphicClass)\n}",
		"func polymorphicClass(p any) (string, bool) {",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("rendered rm hooks file lacks %q", want)
		}
	}
	if decls := regexp.MustCompile(`(?m)^func [A-Z]`).FindAllString(src, -1); len(decls) != 0 {
		t.Errorf("rendered rm hooks file declares exported functions %q; the walker's table must add no public API", decls)
	}

	hooks := regexp.MustCompile(`out \*(\w+)\) error \{\n\t\treturn typereg\.DecodePolymorphic\(dec, out, (nil|func\(\) any \{ return &(\w+)\{\} \})\)`).FindAllStringSubmatch(src, -1)
	if len(hooks) < 10 {
		t.Fatalf("found %d decode hooks in the rendered rm hooks file; want the full set of RM interfaces", len(hooks))
	}
	cases := map[string]string{}
	for _, m := range regexp.MustCompile(`case \*(\w+):\n\t\treturn "(\w+)", true`).FindAllStringSubmatch(src, -1) {
		cases[m[1]] = m[2]
	}
	if len(cases) != len(hooks) {
		t.Errorf("polymorphicClass has %d cases for %d hooked interfaces; want one per interface", len(cases), len(hooks))
	}
	for _, h := range hooks {
		iface, fallbackGo := h[1], h[3]
		class, ok := cases[iface]
		if !ok {
			t.Errorf("polymorphicClass has no case for the hooked interface %s", iface)
			continue
		}
		pc, known := plan.Classes[class]
		switch {
		case !known:
			t.Errorf("%s: class %q is not in the plan", iface, class)
		case fallbackGo != "" && pc.GoName != fallbackGo:
			t.Errorf("%s: class %q is Go type %s, want %s, the parent its hook falls back to", iface, class, pc.GoName, fallbackGo)
		case fallbackGo == "" && pc.GoName != iface:
			t.Errorf("%s: class %q is Go type %s, want the abstract class the interface is generated from", iface, class, pc.GoName)
		}
	}
	for iface, want := range map[string]string{"DataValue": "DATA_VALUE", "DVOrdered": "DV_ORDERED", "DVTextLike": "DV_TEXT"} {
		if got := cases[iface]; got != want {
			t.Errorf("polymorphicClass(*%s) = %q, want %q", iface, got, want)
		}
	}

	aom, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM 1.4): %v", err)
	}
	aomSrc, err := RenderJSONHooksFile(aom)
	if err != nil {
		t.Fatalf("RenderJSONHooksFile(AOM 1.4): %v", err)
	}
	if strings.Contains(string(aomSrc), "polyreg") || strings.Contains(string(aomSrc), "polymorphicClass") {
		t.Error("rendered AOM 1.4 hooks file registers an interface table; the JSON walk does not check AOM 1.4")
	}
}
