package bmmgen

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// The invariant inventory (REQ-171 *Where the RM rules come from*).
//
// [RenderInventoryFile] emits openehr/validation/inventory_gen.go: every
// invariant the pinned RM and BASE BMMs declare, BASE's primitive types
// included, keyed CLASS.Invariant (the declaring class and the invariant's
// name, as the BMM spells them), each with the classification the checked-in
// file openehr/validation/invariants.json gives it; and, for every concrete
// class the type registry decodes, the keys that class is subject to, its own
// and those its ancestors declare. The generator reads only the keys of the
// BMM `invariants` maps, never the expressions.
//
// Every invariant of the pinned BMMs must have a classification, and every
// classification must name an invariant the pinned BMMs declare: either gap
// fails generation, and so `make codegen` and `make codegen-verify`, naming
// the key, so a BMM bump cannot add an unchecked rule silently.

// inventoryFile is the generated inventory, written into the validation
// package.
const inventoryFile = "inventory_gen.go"

// classificationFile is the checked-in, hand-maintained classification of
// the inventory, read from the validation package beside the evaluators it
// names.
const classificationFile = "invariants.json"

// inventoryDir is the validation package, relative to the output module root.
var inventoryDir = filepath.Join("openehr", "validation")

// The three kinds a classification gives an invariant.
const (
	// kindEvaluated is an invariant the SDK evaluates, with the Go function
	// that evaluates it.
	kindEvaluated = "evaluated"
	// kindNotMechanical is an invariant no check on the instance alone can
	// decide: it needs a terminology service, stored history, or a
	// comparison the RM leaves undefined.
	kindNotMechanical = "not_mechanical"
	// kindOutOfScope is an invariant the SDK does not check on an instance
	// for another stated reason.
	kindOutOfScope = "out_of_scope"
)

// kindIdents maps each kind to the Go identifier the hand-written dispatch
// declares for it, so a renamed constant fails to compile in the generated
// file.
var kindIdents = map[string]string{
	kindEvaluated:     "invariantEvaluated",
	kindNotMechanical: "invariantNotMechanical",
	kindOutOfScope:    "invariantOutOfScope",
}

// invariantClassification is one entry of the classification file: an
// evaluated invariant names the Go function that evaluates it; any other
// gives the reason the SDK does not evaluate it.
type invariantClassification struct {
	Kind     string `json:"kind"`
	Function string `json:"function,omitzero"`
	Reason   string `json:"reason,omitzero"`
}

// parseClassification decodes the classification file. It refuses a member it
// does not know, a duplicate key, an unknown kind, an evaluated entry without
// a Go identifier for its function or with a reason, and any other entry
// without a reason or with a function.
func parseClassification(data []byte) (map[string]invariantClassification, error) {
	var out map[string]invariantClassification
	if err := json.Unmarshal(data, &out, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("bmmgen: decode %s: %w", classificationFile, err)
	}
	var problems []string
	for _, key := range sortedStringKeys(out) {
		c := out[key]
		switch c.Kind {
		case kindEvaluated:
			if !token.IsIdentifier(c.Function) {
				problems = append(problems, fmt.Sprintf("%s is evaluated but names no Go function (%q)", key, c.Function))
			}
			if c.Reason != "" {
				problems = append(problems, key+" is evaluated and gives a reason; an evaluated entry names its function only")
			}
		case kindNotMechanical, kindOutOfScope:
			if strings.TrimSpace(c.Reason) == "" {
				problems = append(problems, fmt.Sprintf("%s is %s and gives no reason", key, c.Kind))
			}
			if c.Function != "" {
				problems = append(problems, fmt.Sprintf("%s is %s and names a function; only an evaluated entry does", key, c.Kind))
			}
		default:
			problems = append(problems, fmt.Sprintf("%s has kind %q; want %s, %s or %s", key, c.Kind, kindEvaluated, kindNotMechanical, kindOutOfScope))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("bmmgen: %s: %s", classificationFile, strings.Join(problems, "; "))
	}
	return out, nil
}

// loadSchemaChain loads the schema rootID names and every schema it includes,
// transitively, each on its own and unmerged, so an invariant of an included
// schema whose class the including one redefines is still seen. The schemas
// come back in the order they were reached, rootID first.
func loadSchemaChain(ctx context.Context, rootID string, resolver bmm.Resolver) ([]*bmm.Schema, error) {
	seen := map[string]bool{rootID: true}
	queue := []string{rootID}
	var out []*bmm.Schema
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		rc, err := resolver.Resolve(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("bmmgen: resolve %q: %w", id, err)
		}
		s, err := bmm.Load(rc)
		if closeErr := rc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, fmt.Errorf("bmmgen: load %q: %w", id, err)
		}
		out = append(out, s)
		for _, incl := range sortedStringKeys(s.Includes) {
			if !seen[incl] {
				seen[incl] = true
				queue = append(queue, incl)
			}
		}
	}
	return out, nil
}

// classInvariantNames returns the names of the invariants c declares, sorted.
func classInvariantNames(c bmm.Class) []string {
	var inv map[string]string
	switch x := c.(type) {
	case *bmm.SimpleClass:
		inv = x.Invariants
	case *bmm.Interface:
		inv = x.Invariants
	case *bmm.Enumeration:
		inv = x.Invariants
	}
	return sortedStringKeys(inv)
}

// declaredInvariants returns every invariant the schemas declare, in their
// class definitions and their primitive types, as CLASS.Invariant, sorted and
// once each: two schemas that declare one class with one invariant name
// (BASE's CODE_PHRASE and the RM's) give one key.
func declaredInvariants(schemas []*bmm.Schema) ([]string, error) {
	seen := map[string]bool{}
	for _, s := range schemas {
		for _, defs := range []map[string]bmm.Class{s.ClassDefinitions, s.PrimitiveTypes} {
			for _, class := range sortedStringKeys(defs) {
				for _, name := range classInvariantNames(defs[class]) {
					if name == "" {
						return nil, fmt.Errorf("bmmgen: %s declares an invariant with no name in %s", class, s.SchemaID())
					}
					seen[class+"."+name] = true
				}
			}
		}
	}
	return sortedStringKeys(seen), nil
}

// schemaClass returns the class name declares in the merged schema, among its
// class definitions or its primitive types.
func schemaClass(s *bmm.Schema, name string) (bmm.Class, bool) {
	if c, ok := s.ClassDefinitions[name]; ok {
		return c, true
	}
	c, ok := s.PrimitiveTypes[name]
	return c, ok
}

// inventoryAncestry returns name and every class it descends from in the
// merged schema, transitively. An ancestor the schema does not declare fails,
// since its invariants could not be read.
func inventoryAncestry(s *bmm.Schema, name string) ([]string, error) {
	seen := map[string]bool{name: true}
	out := []string{name}
	for i := 0; i < len(out); i++ {
		c, ok := schemaClass(s, out[i])
		if !ok {
			return nil, fmt.Errorf("bmmgen: invariant inventory: %s descends from %s, which the pinned BMMs do not declare", name, out[i])
		}
		for _, anc := range c.Ancestors() {
			anc, _, _ = strings.Cut(anc, "<")
			if !seen[anc] {
				seen[anc] = true
				out = append(out, anc)
			}
		}
	}
	return out, nil
}

// checkClassified reports every invariant of declared that classes does not
// classify, and every classification that names no declared invariant, each
// by its key.
func checkClassified(declared []string, classes map[string]invariantClassification) error {
	inBMM := make(map[string]bool, len(declared))
	var missing, dead []string
	for _, key := range declared {
		inBMM[key] = true
		if _, ok := classes[key]; !ok {
			missing = append(missing, key)
		}
	}
	for _, key := range sortedStringKeys(classes) {
		if !inBMM[key] {
			dead = append(dead, key)
		}
	}
	var errs []error
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("bmmgen: %d invariants of the pinned BMMs have no classification in %s; classify each as %s, %s or %s: %s",
			len(missing), classificationFile, kindEvaluated, kindNotMechanical, kindOutOfScope, strings.Join(missing, ", ")))
	}
	if len(dead) > 0 {
		errs = append(errs, fmt.Errorf("bmmgen: %s classifies %d invariants no pinned BMM declares; remove them: %s",
			classificationFile, len(dead), strings.Join(dead, ", ")))
	}
	return errors.Join(errs...)
}

// RenderInventoryFile produces openehr/validation/inventory_gen.go from a
// planned RM emission, the invariants its schemas declare (declared, from
// [declaredInvariants]) and their classification. It fails when an invariant
// of declared has no classification or a classification names no invariant
// of declared, naming each key.
//
// It emits invariantInventory, every key of declared with its kind and, for
// an evaluated one, the function the classification names; and
// classInvariants, for every class in [Plan.ConcreteClasses], the keys of the
// invariants the class and its ancestors in the merged schema declare, sorted
// (nil for a class subject to none).
//
// Returns nil for every target but RM. Output is deterministic and gofmt'd.
func RenderInventoryFile(plan *Plan, declared []string, classes map[string]invariantClassification) ([]byte, error) {
	if plan.Target.GoPackage != "rm" {
		return nil, nil
	}
	if err := checkClassified(declared, classes); err != nil {
		return nil, err
	}
	byClass := map[string][]string{}
	for _, key := range declared {
		class, _, _ := strings.Cut(key, ".")
		byClass[class] = append(byClass[class], key)
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by bmmgen; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// Source: %s, classified in %s\n\n", plan.Target.SourceLabel, classificationFile)
	b.WriteString("package validation\n\n")
	b.WriteString("// invariantInventory holds every invariant the pinned RM and BASE BMMs\n")
	b.WriteString("// declare, BASE's primitive types included, keyed CLASS.Invariant, with its\n")
	fmt.Fprintf(&b, "// kind in %s and, for an evaluated one, the Go function that\n", classificationFile)
	b.WriteString("// evaluates it.\n")
	b.WriteString("var invariantInventory = map[string]inventoryEntry{\n")
	for _, key := range declared {
		c := classes[key]
		if c.Kind == kindEvaluated {
			fmt.Fprintf(&b, "\t%q: {kind: %s, function: %q},\n", key, kindIdents[c.Kind], c.Function)
			continue
		}
		fmt.Fprintf(&b, "\t%q: {kind: %s},\n", key, kindIdents[c.Kind])
	}
	b.WriteString("}\n\n")
	b.WriteString("// classInvariants gives, for every concrete class the type registry decodes,\n")
	b.WriteString("// the keys of invariantInventory the class is subject to: those it declares\n")
	b.WriteString("// and those its ancestors declare, sorted. A class subject to none maps to\n")
	b.WriteString("// nil.\n")
	b.WriteString("var classInvariants = map[string][]string{\n")
	for _, pc := range plan.ConcreteClasses {
		ancestry, err := inventoryAncestry(plan.Schema, pc.BMMName)
		if err != nil {
			return nil, err
		}
		var keys []string
		for _, class := range ancestry {
			keys = append(keys, byClass[class]...)
		}
		slices.Sort(keys)
		if len(keys) == 0 {
			fmt.Fprintf(&b, "\t%q: nil,\n", pc.BMMName)
			continue
		}
		fmt.Fprintf(&b, "\t%q: {\n", pc.BMMName)
		for _, key := range keys {
			fmt.Fprintf(&b, "\t\t%q,\n", key)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gofmt %s: %w", inventoryFile, err)
	}
	return formatted, nil
}

// emitInventoryFile renders the inventory for plan and writes it into the
// validation package under the output root, or, in verify mode, records drift
// against what is on disk. It reads the classification from that package.
//
// It is a no-op for a target [RenderInventoryFile] does not emit for, and for
// an output tree that holds no validation package, such as the scratch tree a
// generator test writes into. In a tree that holds one, a missing
// classification file fails.
func emitInventoryFile(opts Options, plan *Plan, resolver bmm.Resolver, tr *TargetResult, result *Result) error {
	if plan.Target.GoPackage != "rm" {
		return nil
	}
	dir := filepath.Join(opts.OutDir, inventoryDir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("bmmgen: stat %s: %w", dir, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, classificationFile))
	if err != nil {
		return fmt.Errorf("bmmgen: read the invariant classification: %w", err)
	}
	classes, err := parseClassification(data)
	if err != nil {
		return err
	}
	schemas, err := loadSchemaChain(context.Background(), plan.Target.RootID, resolver)
	if err != nil {
		return err
	}
	declared, err := declaredInvariants(schemas)
	if err != nil {
		return err
	}
	body, err := RenderInventoryFile(plan, declared, classes)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, inventoryFile)
	tr.Files = append(tr.Files, path)
	result.Files = append(result.Files, path)
	if opts.Verify {
		drift, err := compareFile(path, body)
		if err != nil {
			return err
		}
		if drift != nil {
			result.Drifts = append(result.Drifts, *drift)
		}
		return nil
	}
	return writeAtomic(path, body)
}
