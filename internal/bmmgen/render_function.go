package bmmgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// funcEmitContext carries the metadata needed to emit a single
// method stub. owner is the BMM SimpleClass that originally declared
// the function (used for parameter / result type resolution). recv
// is the Go class whose receiver gets the method (may differ from
// owner: a function declared on an abstract class is propagated to
// every concrete descendant, with the descendant as the receiver but
// the original owner as the resolution scope for open generics).
type funcEmitContext struct {
	plan      *Plan
	ownerName string // BMM name of the declaring class (for panic message + open-generic scope)
	owner     *bmm.SimpleClass
	recv      *PlannedClass    // PlannedClass whose receiver we attach to
	recvClass *bmm.SimpleClass // BMM SimpleClass for the receiver (for generic receiver params)
	fn        *bmm.Function
}

// receiverIdent returns the receiver-variable identifier for a class
// receiver. Uses the lower-cased first letter of the Go class name,
// falling back to "x" for empty names. Keeping the per-class
// identifier (vs always `c`) prevents collisions with parameters
// named `c` (common — e.g. `TERM_MAPPING.is_valid_match_code(c)`).
func receiverIdent(goName string) string {
	if goName == "" {
		return "x"
	}
	r := []rune(goName)
	return strings.ToLower(string(r[0]))
}

// renderFunctions emits all functions declared on sc (the planned
// class's BMM class) as Go method stubs. For abstract non-generic
// classes (rendered as Go interfaces), the methods are emitted on
// each concrete descendant that does NOT itself override the function
// — the abstract Go interface stays marker-only. For abstract+generic
// classes (rendered as Go structs), the methods are emitted directly
// on the struct; descendants embed and inherit them, with override
// support via Go method shadowing.
//
// Method counts (emitted, TODO escapes) are tallied into the supplied
// Plan via [Plan.MethodStubsEmitted] / [Plan.MethodTodoEscapes].
func renderFunctions(b *strings.Builder, plan *Plan, pc *PlannedClass) error {
	sc, isSimple := pc.Class.(*bmm.SimpleClass)
	if !isSimple {
		return nil
	}
	if len(sc.Functions) == 0 {
		return nil
	}

	if sc.IsAbstract() && (!sc.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc)) {
		// Abstract class (non-generic or codec-polymorphic generic):
		// emit on every concrete descendant
		// that does not itself declare the function. Iterate in BMM-name
		// sort order for determinism.
		//
		// Deduplication: a single descendant may transitively inherit
		// the same function name from multiple abstract ancestors
		// (e.g. DV_QUANTIFIED + Any both declare `is_equal`). The
		// closest-ancestor-wins claim set is computed for each
		// descendant — only emit here if the function-name's nearest
		// declaring ancestor is THIS class.
		fnNames := sortedStringKeys(sc.Functions)
		descendants := plan.AbstractDescendants[pc.BMMName]
		for _, fnName := range fnNames {
			fn := sc.Functions[fnName]
			for _, dName := range descendants {
				dpc, ok := plan.Classes[dName]
				if !ok {
					continue
				}
				dsc, isSimple := dpc.Class.(*bmm.SimpleClass)
				if !isSimple {
					continue
				}
				if _, overrides := dsc.Functions[fnName]; overrides {
					// Descendant declares its own version — the descendant's
					// renderFunctions call emits it.
					continue
				}
				// Closest-ancestor-wins: skip if an ancestor closer to
				// the descendant than pc also declares this function.
				if nearestDeclarer(plan, dsc, fnName) != pc.BMMName {
					continue
				}
				// Skip if the receiver already has a property/field with
				// the PascalCased Go name (avoids field-vs-method
				// collisions — the property takes precedence).
				if descendantHasFieldNamed(plan, dsc, MethodName(fnName)) {
					continue
				}
				ctx := funcEmitContext{
					plan:      plan,
					ownerName: pc.BMMName,
					owner:     sc,
					recv:      dpc,
					recvClass: dsc,
					fn:        fn,
				}
				chunk, td, err := emitMethodStub(ctx)
				if err != nil {
					return err
				}
				plan.MethodStubsEmitted++
				plan.MethodTodoEscapes += td
				b.WriteString("\n")
				b.WriteString(chunk)
			}
		}
		return nil
	}

	// Concrete OR abstract+generic class: emit functions directly on
	// this class's receiver. Iterate in BMM-name sort order.
	for _, fnName := range sortedStringKeys(sc.Functions) {
		fn := sc.Functions[fnName]
		// Skip if there's a same-named property on this class — the
		// field already provides the accessor surface (Go forbids
		// method-on-struct sharing the name of a struct field).
		if descendantHasFieldNamed(plan, sc, MethodName(fnName)) {
			continue
		}
		ctx := funcEmitContext{
			plan:      plan,
			ownerName: pc.BMMName,
			owner:     sc,
			recv:      pc,
			recvClass: sc,
			fn:        fn,
		}
		chunk, td, err := emitMethodStub(ctx)
		if err != nil {
			return err
		}
		plan.MethodStubsEmitted++
		plan.MethodTodoEscapes += td
		b.WriteString("\n")
		b.WriteString(chunk)
	}
	return nil
}

// nearestDeclarer walks the descendant's ancestor chain (closest
// first) and returns the BMM name of the first ancestor that
// declares the given function. Returns "" if no ancestor declares
// it. Used for closest-ancestor-wins deduplication: when a function
// is declared in multiple abstract ancestors, only the closest one
// emits the propagated stub.
func nearestDeclarer(plan *Plan, dsc *bmm.SimpleClass, fnName string) string {
	visited := map[string]bool{}
	// Walk ancestors in declaration order, BFS — gives us closest-first.
	queue := append([]string{}, dsc.Ancestors()...)
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if visited[next] {
			continue
		}
		visited[next] = true
		ap, ok := plan.Classes[next]
		if !ok {
			continue
		}
		asc, isSimple := ap.Class.(*bmm.SimpleClass)
		if !isSimple {
			continue
		}
		if _, has := asc.Functions[fnName]; has {
			return next
		}
		queue = append(queue, asc.Ancestors()...)
	}
	return ""
}

// descendantHasFieldNamed reports whether the descendant SimpleClass
// (own + transitively flattened abstract-ancestor properties) has a
// property whose Go field name equals the supplied PascalCase name.
// Used to skip method stubs that would collide with a field.
func descendantHasFieldNamed(plan *Plan, dsc *bmm.SimpleClass, goFieldName string) bool {
	for _, p := range dsc.Properties {
		if FieldName(p.PropertyName()) == goFieldName {
			return true
		}
	}
	// Walk abstract ancestors.
	visited := map[string]bool{}
	var rec func(c *bmm.SimpleClass) bool
	rec = func(c *bmm.SimpleClass) bool {
		for _, anc := range c.Ancestors() {
			if visited[anc] {
				continue
			}
			visited[anc] = true
			ap, ok := plan.Classes[anc]
			if !ok {
				continue
			}
			asc, isSimple := ap.Class.(*bmm.SimpleClass)
			if !isSimple {
				continue
			}
			if !asc.IsAbstract() {
				continue
			}
			for _, p := range asc.Properties {
				if FieldName(p.PropertyName()) == goFieldName {
					return true
				}
			}
			if rec(asc) {
				return true
			}
		}
		return false
	}
	return rec(dsc)
}

// emitMethodStub returns the Go source for one method stub: doc
// comment block (with Pre/Post/Aliases), declaration, and a single-
// line panic body.
//
// The receiver expression is `(c *<RecvGoName>[Params])` — pointer
// receiver per the Phase 3 spec ("always use `c` for this concrete
// class instance"). Generic receiver parameters are appended for
// generic classes.
func emitMethodStub(ctx funcEmitContext) (string, int, error) {
	goMethod := MethodName(ctx.fn.Name)
	doc := methodDocBlock(goMethod, ctx.fn)

	// Receiver expression. Use the lower-cased first letter of the
	// class name as the receiver variable so parameter names like
	// `c` (e.g. TERM_MAPPING.is_valid_match_code(c)) don't collide
	// with the receiver identifier.
	recvVar := receiverIdent(ctx.recv.GoName)
	recvGenerics := genericReceiverParams(ctx.recvClass)
	receiver := fmt.Sprintf("(%s *%s%s)", recvVar, ctx.recv.GoName, recvGenerics)

	// Parameter list (sorted by parameter name). Pass the reserved
	// receiver variable so any colliding parameter gets renamed.
	paramList, err := renderFunctionParameters(ctx, recvVar)
	if err != nil {
		return "", 0, err
	}

	// Result type (may be empty for void).
	resultType, todo, err := renderFunctionResult(ctx)
	if err != nil {
		return "", 0, err
	}

	resultPart := ""
	if resultType != "" {
		resultPart = " " + resultType
	}

	// Panic body uses the BMM names (ALL_CAPS class, snake_case fn) so
	// the message lines up with the spec when a developer greps for it.
	body := fmt.Sprintf("\tpanic(%q)\n",
		fmt.Sprintf("not implemented: %s.%s — implement in a non-generated file", ctx.ownerName, ctx.fn.Name))

	var b strings.Builder
	b.WriteString(doc)
	fmt.Fprintf(&b, "func %s %s(%s)%s {\n", receiver, goMethod, paramList, resultPart)
	b.WriteString(body)
	b.WriteString("}\n")
	return b.String(), todo, nil
}

// renderFunctionParameters formats a function's BMM parameters as
// a comma-separated Go parameter list (`name Type, name Type`).
// Parameters are sorted by BMM name for determinism. Empty for
// functions with no parameters. reservedRecv is the receiver
// variable name; any parameter whose Go identifier collides gets a
// trailing underscore so it does not shadow the receiver.
func renderFunctionParameters(ctx funcEmitContext, reservedRecv string) (string, error) {
	if len(ctx.fn.Parameters) == 0 {
		return "", nil
	}
	names := sortedStringKeys(ctx.fn.Parameters)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		p := ctx.fn.Parameters[n]
		typ, err := functionParameterType(ctx, p)
		if err != nil {
			return "", err
		}
		pn := ParamName(n)
		if pn == reservedRecv {
			pn += "_"
		}
		parts = append(parts, fmt.Sprintf("%s %s", pn, typ))
	}
	return strings.Join(parts, ", "), nil
}

// functionParameterType maps a single FunctionParameter to its Go
// type expression. Mirrors the property-side dispatch in renderField.
func functionParameterType(ctx funcEmitContext, p bmm.FunctionParameter) (string, error) {
	switch v := p.(type) {
	case *bmm.SingleFunctionParameter:
		typ, _, err := singleTypeRef(ctx.plan, ctx.owner, v.TypeName)
		return typ, err
	case *bmm.SingleFunctionParameterOpen:
		// Open generic parameter: name is a class-level type parameter
		// on the owner. Emit verbatim.
		return v.TypeName, nil
	case *bmm.ContainerFunctionParameter:
		inner, err := containerInner(ctx.plan, ctx.owner, v.TypeDef)
		if err != nil {
			return "", err
		}
		if v.TypeDef == nil {
			return "[]" + inner, nil
		}
		switch v.TypeDef.ContainerType {
		case "Hash":
			return "map[string]" + inner, nil
		default:
			return "[]" + inner, nil
		}
	case *bmm.GenericFunctionParameter:
		return genericTypeRef(ctx.plan, ctx.owner, v.TypeDef)
	default:
		return "", fmt.Errorf("unhandled FunctionParameter kind %T", p)
	}
}

// renderFunctionResult returns the Go return type expression for the
// function. Empty string means a void method (no return type). The
// `todo` counter increases when the result resolution falls back to
// `any` because the underlying class is skipped / unmapped.
func renderFunctionResult(ctx funcEmitContext) (string, int, error) {
	if ctx.fn.Result == nil {
		return "", 0, nil
	}
	typ, err := typeRef(ctx.plan, ctx.owner, ctx.fn.Result)
	if err != nil {
		return "", 0, err
	}
	if strings.Contains(typ, "TODO") {
		// typeRef inserts inline `any /* TODO: ... */` markers when it
		// can't resolve; that's not valid as a return type expression
		// because of the inline comment. Strip to plain `any` and count
		// the escape.
		return "any", 1, nil
	}
	return typ, 0, nil
}

// flattenExpr collapses a BMM OCL-ish expression onto a single line:
// CR/LF sequences are normalised to single spaces, runs of internal
// whitespace are collapsed, and leading/trailing whitespace is
// trimmed. Used for `// Pre:` / `// Post:` comment lines so an
// embedded newline in the BMM pre/post string does not produce
// broken Go syntax.
func flattenExpr(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Collapse multiple spaces.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// methodDocBlock renders the doc comment block above a method:
//
//   - First line: `// <GoMethodName> <first BMM doc paragraph>`. If the
//     BMM documentation is empty, emit `// <GoMethodName> (no BMM
//     documentation).`
//   - Subsequent BMM-doc paragraphs preserved as `//`-prefixed lines.
//   - One `// Pre: <expr>` line per pre-condition (sorted by key).
//   - One `// Post: <expr>` line per post-condition (sorted by key).
//   - `// Aliases: a, b (Go does not support operator overloading)`
//     when aliases are non-empty.
func methodDocBlock(goMethod string, fn *bmm.Function) string {
	var b strings.Builder
	doc := cleanDoc(fn.Documentation)
	if doc == "" {
		fmt.Fprintf(&b, "// %s (no BMM documentation).\n", goMethod)
	} else {
		lines := strings.Split(doc, "\n")
		fmt.Fprintf(&b, "// %s %s\n", goMethod, lines[0])
		for _, l := range lines[1:] {
			if l == "" {
				b.WriteString("//\n")
			} else {
				fmt.Fprintf(&b, "// %s\n", l)
			}
		}
	}

	// Pre-conditions, sorted by key.
	if len(fn.PreConditions) > 0 {
		// Blank separator if there was real documentation above.
		if doc != "" {
			b.WriteString("//\n")
		}
		keys := sortedStringKeys(fn.PreConditions)
		for _, k := range keys {
			fmt.Fprintf(&b, "// Pre: %s\n", flattenExpr(fn.PreConditions[k]))
		}
	}

	// Post-conditions, sorted by key.
	if len(fn.PostConditions) > 0 {
		if len(fn.PreConditions) == 0 && doc != "" {
			b.WriteString("//\n")
		}
		keys := sortedStringKeys(fn.PostConditions)
		for _, k := range keys {
			fmt.Fprintf(&b, "// Post: %s\n", flattenExpr(fn.PostConditions[k]))
		}
	}

	// Aliases — note Go has no operator overloading.
	if len(fn.Aliases) > 0 {
		if doc != "" || len(fn.PreConditions) > 0 || len(fn.PostConditions) > 0 {
			b.WriteString("//\n")
		}
		aliases := append([]string(nil), fn.Aliases...)
		sort.Strings(aliases)
		fmt.Fprintf(&b, "// Aliases: %s (Go does not support operator overloading)\n",
			strings.Join(aliases, ", "))
	}
	return b.String()
}
