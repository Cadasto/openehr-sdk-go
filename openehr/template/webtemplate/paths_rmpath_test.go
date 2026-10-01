package webtemplate_test

// REQ-121 / REQ-053 — every Web Template node's aqlPath resolves through
// rmpath.
//
// The FLAT encoder walks the Web Template and reads each node's value through
// rmpath, and it reads "path not found" as an absent optional. So a node whose
// path rmpath cannot navigate is data the encoder drops without a word: the
// ACTION `ism_transition` it once dropped is the instance that motivated this
// guard. TestInContextLeavesResolveViaRmpath covers the leaves the builder
// synthesises from the RM; this guard covers every node, archetyped or not,
// that the builder emits for any vendored OPT.
//
// For each node it builds an RM instance along the node's own path, one object
// per segment, and asserts rmpath.ItemsAtPath finds something at the path.
// Building the instance uses reflection, which is a test's freedom: rmpath
// itself stays reflection-free (REQ-121).

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// ctxOwnedPaths are the Web Template paths rmpath deliberately leaves
// unresolved, each with its reason. They are the composition metadata the ctx/
// short forms carry on encode (ADR 0015 makes encode ctx/-only), so resolving
// them would spell one value under two keys. The exemption is by path, not by
// template: an entry no vendored template reaches is reported as stale.
var ctxOwnedPaths = map[string]string{
	"/language":           "COMPOSITION.language is carried by ctx/language on encode",
	"/territory":          "COMPOSITION.territory is carried by ctx/territory on encode",
	"/context/start_time": "EVENT_CONTEXT.start_time is carried by ctx/time on encode",
	"/context/setting":    "EVENT_CONTEXT.setting is carried by ctx/setting|code + |value on encode",
}

// derivedAttributes are RM attributes a template may constrain but an instance
// does not store, keyed OWNER.attr, each with its reason. There is nothing at
// such a node for rmpath to resolve, and so nothing the encoder can lose there.
var derivedAttributes = map[string]string{
	"POINT_EVENT.offset": "EVENT.offset is an RM function (time minus HISTORY.origin), not a stored " +
		"attribute: the value lives in the event's time and the history's origin",
}

// errDerived reports a path that reaches one of the derivedAttributes.
type errDerived struct{ key string }

func (e errDerived) Error() string { return e.key + " is derived, not stored" }

// encodedRoot is the one root class the FLAT encoder reaches: MarshalFlat
// takes a *rm.Composition. A Web Template rooted anywhere else (a demographic
// PERSON or ADDRESS) is outside the encoder's reach, so its paths cannot be a
// silent encode loss, and the guard skips it by name.
const encodedRoot = "COMPOSITION"

// vendoredOPTs lists every OPT this repository vendors: the template corpus and
// the PROBE-086 FLAT conformance template.
func vendoredOPTs(t *testing.T) []string {
	t.Helper()
	corpus, err := filepath.Glob("../../../testkit/corpus/templates/*.opt")
	if err != nil {
		t.Fatal(err)
	}
	flat, err := filepath.Glob("../../../testkit/corpus/flat-conformance/templates/*.opt")
	if err != nil {
		t.Fatal(err)
	}
	all := append(corpus, flat...)
	if len(all) == 0 {
		t.Fatal("no vendored OPT found — corpus layout changed?")
	}
	slices.Sort(all)
	return all
}

// TestWebTemplatePathsResolveViaRmpath — REQ-121, REQ-053. For every node of
// the Web Template built from every vendored OPT rooted at a COMPOSITION,
// rmpath resolves the node's aqlPath on an instance built along it, unless the
// path is ctx/-owned or reaches a derived attribute.
func TestWebTemplatePathsResolveViaRmpath(t *testing.T) {
	exempted := map[string]bool{}
	for _, path := range vendoredOPTs(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			c, w, skip := buildVendored(path)
			if skip != "" {
				t.Skip(skip)
			}
			if w.Tree.RMType != encodedRoot {
				t.Skipf("rooted at %s: the FLAT encoder takes a COMPOSITION, so these paths never reach it "+
					"(rmpath does not navigate the demographic classes at all)", w.Tree.RMType)
			}
			var nodes []*webtemplate.Node
			collectNodes(w.Tree, &nodes)
			for _, n := range nodes {
				p := barePath(n.AQLPath)
				if _, owned := ctxOwnedPaths[p]; owned {
					exempted[p] = true
					continue
				}
				root, err := instanceAlong(c, w.Tree.RMType, n, p)
				if derived, ok := errors.AsType[errDerived](err); ok {
					exempted[derived.key] = true
					continue
				}
				if err != nil {
					t.Errorf("%s (%s): cannot build an instance along the path: %v", n.AQLPath, n.RMType, err)
					continue
				}
				items, err := rmpath.ItemsAtPath(root, p)
				if err != nil {
					t.Errorf("%s (%s): rmpath: %v", n.AQLPath, n.RMType, err)
					continue
				}
				if len(items) == 0 {
					t.Errorf("%s (%s): rmpath resolves nothing on an instance built along the path — "+
						"the FLAT encoder would drop a value here without an error. Add the attribute to "+
						"rmpath's childrenAt (%s), or, if it must stay unresolved, exempt it here with the reason",
						n.AQLPath, n.RMType, unresolvedStep(root, p))
				}
			}
		})
	}
	for p, why := range ctxOwnedPaths {
		if !exempted[p] {
			t.Errorf("stale ctxOwnedPaths entry %q (%s): no vendored Web Template has a node there", p, why)
		}
	}
	for k, why := range derivedAttributes {
		if !exempted[k] {
			t.Errorf("stale derivedAttributes entry %q (%s): no vendored Web Template reaches it", k, why)
		}
	}
}

// buildVendored parses, compiles and builds one OPT. A non-empty skip names the
// step that refused it and why, so a refused template is visible in the output
// rather than silently absent.
func buildVendored(path string) (*templatecompile.Compiled, *webtemplate.WebTemplate, string) {
	opt, err := template.ParseFile(path)
	if err != nil {
		return nil, nil, fmt.Sprintf("the OPT parser refuses %s: %v", filepath.Base(path), err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, nil, fmt.Sprintf("the template compiler refuses %s: %v", filepath.Base(path), err)
	}
	w, err := webtemplate.Build(c)
	if err != nil {
		return nil, nil, fmt.Sprintf("the Web Template builder refuses %s: %v", filepath.Base(path), err)
	}
	return c, w, ""
}

// collectNodes lists every node below the root.
func collectNodes(n *webtemplate.Node, out *[]*webtemplate.Node) {
	for _, ch := range n.Children {
		*out = append(*out, ch)
		collectNodes(ch, out)
	}
}

// unresolvedStep names the first path prefix rmpath stops resolving at, as
// OWNER.attr, so a failure points at the missing childrenAt case.
func unresolvedStep(root rm.Locatable, p string) string {
	segs := splitPath(p)
	var prefix strings.Builder
	owner, _ := rm.RMTypeName(root)
	for _, s := range segs {
		prefix.WriteString("/" + s.raw)
		items, err := rmpath.ItemsAtPath(root, prefix.String())
		if err != nil || len(items) == 0 {
			if s.pred != "" {
				return fmt.Sprintf("%s.%s, or its [%s] predicate", owner, s.attr, s.pred)
			}
			return owner + "." + s.attr
		}
		owner, _ = rm.RMTypeName(items[0])
	}
	return "no step"
}

// pathSeg is one segment of a bare aqlPath.
type pathSeg struct {
	raw, attr, pred string
}

// splitPath splits a bare aqlPath at the slashes outside predicates.
func splitPath(p string) []pathSeg {
	var out []pathSeg
	depth, start := 0, 0
	p = strings.TrimPrefix(p, "/")
	flush := func(raw string) {
		s := pathSeg{raw: raw, attr: raw}
		if i := strings.IndexByte(raw, '['); i >= 0 && strings.HasSuffix(raw, "]") {
			s.attr, s.pred = raw[:i], raw[i+1:len(raw)-1]
		}
		out = append(out, s)
	}
	for i := range len(p) {
		switch p[i] {
		case '[':
			depth++
		case ']':
			depth--
		case '/':
			if depth == 0 {
				flush(p[start:i])
				start = i + 1
			}
		}
	}
	if start < len(p) {
		flush(p[start:])
	}
	return out
}

// barePath strips the REQ-116 name predicates (`[at0001,'Name']` →
// `[at0001]`), as the FLAT encoder does before it resolves a node: the
// instance built here is matched by archetype_node_id alone.
func barePath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] == ',' && i+1 < len(p) && p[i+1] == '\'' {
			j := i + 2
			for j < len(p) {
				if p[j] == '\\' && j+1 < len(p) {
					j += 2
					continue
				}
				if p[j] == '\'' {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(p[i])
		i++
	}
	return b.String()
}

// bareClass drops a generic parameter list: typereg and rminfo know each class
// by its bare BMM name.
func bareClass(name string) string {
	class, _, _ := strings.Cut(strings.TrimSpace(name), "<")
	return strings.TrimSpace(class)
}

// instanceAlong builds a COMPOSITION holding one object per segment of p, each
// of the class the compiled template (or, off the compiled tree, the RM) gives
// that position, and each carrying the identity its predicate names. The last
// object is the node's own value.
func instanceAlong(c *templatecompile.Compiled, rootType string, n *webtemplate.Node, p string) (rm.Locatable, error) {
	ctor, ok := typereg.Default.Lookup(bareClass(rootType))
	if !ok {
		return nil, fmt.Errorf("no registered RM class %q for the root", rootType)
	}
	root, ok := ctor().(rm.Locatable)
	if !ok {
		return nil, fmt.Errorf("the root %q is no LOCATABLE", rootType)
	}
	cur := reflect.ValueOf(root)
	cn := c.Root()
	segs := splitPath(p)
	for i, s := range segs {
		last := i == len(segs)-1
		owner, _ := rm.RMTypeName(cur.Interface())
		next := compiledChild(cn, s, last, n.RMType)
		class := ""
		switch {
		case last:
			class = bareClass(n.RMType)
		case next != nil:
			class = bareClass(next.RMTypeName())
		}
		if class == "" {
			declared, ok := rminfo.Default.AttributeRMType(owner, s.attr)
			if !ok {
				return nil, fmt.Errorf("%s declares no attribute %q", owner, s.attr)
			}
			class = bareClass(declared)
		}
		if _, derived := derivedAttributes[owner+"."+s.attr]; derived {
			return nil, errDerived{key: owner + "." + s.attr}
		}
		child, err := setAttr(cur, s.attr, class)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", owner, s.attr, err)
		}
		if s.pred != "" {
			identify(child, s.pred)
		}
		cur, cn = child, next
	}
	return root, nil
}

// compiledChild finds the compiled node a segment reaches from cn, or nil when
// the segment leaves the compiled tree (an in-context attribute the OPT does
// not constrain) or cannot be told apart from its siblings.
func compiledChild(cn *templatecompile.CompiledNode, s pathSeg, last bool, leafType string) *templatecompile.CompiledNode {
	if cn == nil {
		return nil
	}
	a := cn.Attribute(s.attr)
	if a == nil {
		return nil
	}
	kids := a.Children()
	if s.pred != "" {
		for _, k := range kids {
			if k.NodeID() == s.pred || k.ArchetypeID() == s.pred {
				return k
			}
		}
		return nil
	}
	if last {
		for _, k := range kids {
			if bareClass(k.RMTypeName()) == bareClass(leafType) {
				return k
			}
		}
	}
	if len(kids) == 1 {
		return kids[0]
	}
	return nil
}

// setAttr gives the struct owner points at a value of class at the RM attribute
// attr, and returns a pointer to that value so the next segment can descend
// into it.
func setAttr(owner reflect.Value, attr, class string) (reflect.Value, error) {
	s := owner.Elem()
	if s.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("owner is a %s, not an RM object", s.Kind())
	}
	f, ok := fieldByJSON(s.Type(), attr)
	if !ok {
		return reflect.Value{}, fmt.Errorf("%s has no field for the RM attribute", s.Type())
	}
	fv := s.FieldByIndex(f.Index)
	switch fv.Kind() {
	case reflect.Interface:
		v, err := concreteFor(fv.Type(), class)
		if err != nil {
			return reflect.Value{}, err
		}
		fv.Set(v)
		return v, nil
	case reflect.Pointer:
		// An earlier segment may have set it already (an ISM_TRANSITION's
		// careflow_step carries the transition's identity): keep that value.
		if !fv.IsNil() {
			return fv, nil
		}
		v := reflect.New(fv.Type().Elem())
		fv.Set(v)
		return v, nil
	case reflect.Slice:
		et := fv.Type().Elem()
		switch et.Kind() {
		case reflect.Interface:
			v, err := concreteFor(et, class)
			if err != nil {
				return reflect.Value{}, err
			}
			fv.Set(reflect.Append(fv, v))
			return v, nil
		case reflect.Pointer:
			v := reflect.New(et.Elem())
			fv.Set(reflect.Append(fv, v))
			return v, nil
		default:
			fv.Set(reflect.Append(fv, reflect.Zero(et)))
			return fv.Index(fv.Len() - 1).Addr(), nil
		}
	case reflect.String:
		fv.SetString("x")
		return fv.Addr(), nil
	default:
		// A struct held by value, or a scalar: the field already exists, and
		// rmpath hands a by-value attribute back whatever it holds.
		return fv.Addr(), nil
	}
}

// fieldByJSON finds the (possibly promoted) field whose JSON name is attr.
func fieldByJSON(t reflect.Type, attr string) (reflect.StructField, bool) {
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == attr {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// concreteFor returns a new value of class that fits the interface type iface,
// or, when class is abstract or does not fit, of the first registered RM class
// (by name) that does.
func concreteFor(iface reflect.Type, class string) (reflect.Value, error) {
	if ctor, ok := typereg.Default.Lookup(class); ok {
		if v := reflect.ValueOf(ctor()); v.Type().AssignableTo(iface) {
			return v, nil
		}
	}
	for _, name := range slices.Sorted(slices.Values(typereg.Default.Names())) {
		ctor, _ := typereg.Default.Lookup(name)
		if v := reflect.ValueOf(ctor()); v.Type().AssignableTo(iface) {
			return v, nil
		}
	}
	return reflect.Value{}, fmt.Errorf("no registered RM class fits %s (wanted %s)", iface, cmp.Or(class, "any"))
}

// identify gives the object the identity a node-id predicate names: its
// archetype_node_id on a LOCATABLE, and on an ISM_TRANSITION, which carries no
// archetype_node_id, the careflow_step code rmpath matches instead.
func identify(v reflect.Value, pred string) {
	switch x := v.Interface().(type) {
	case *rm.IsmTransition:
		x.CareflowStep = &rm.DVCodedText{
			Value:        "step",
			DefiningCode: rm.CodePhrase{CodeString: pred, TerminologyID: rm.TerminologyID{Value: "local"}},
		}
	case rm.Locatable:
		if f := v.Elem().FieldByName("ArchetypeNodeID"); f.IsValid() && f.CanSet() {
			f.SetString(pred)
		}
	}
}
