package bmmgen

// REQ-041 § SDK-authored root schemas — the generator reads a target's root
// schema from resources/bmm/sdk/ and the pinned schemas the root includes from
// resources/bmm/, and a root never takes a pinned schema's id.

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// testResolver is the resolver the generator builds over the vendored
// resources: the SDK-authored roots first, the pinned schemas second.
func testResolver() bmm.Resolver {
	return NewResolver(testResources)
}

// writeSchemaFiles writes each body to dir/<path>, creating the directories
// the path names.
func writeSchemaFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", path, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// resolveBody resolves id through r and returns the bytes it hands back.
func resolveBody(r bmm.Resolver, id string) (string, error) {
	rc, err := r.Resolve(context.Background(), id)
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(rc)
	if closeErr := rc.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return string(b), err
}

// TestSchemaResolver_REQ041_ReadsTheRootsBeforeThePinnedSchemas — an id the
// roots directory holds resolves there; any other id resolves among the pinned
// schemas; an id neither holds is a missing schema.
func TestSchemaResolver_REQ041_ReadsTheRootsBeforeThePinnedSchemas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSchemaFiles(t, dir, map[string]string{
		"pinned_1.0.0.bmm.json":               "pinned",
		rootsDirName + "/root_1.0.0.bmm.json": "root",
	})
	r := newSchemaResolver(dir)
	for _, tc := range []struct{ id, want string }{
		{"root_1.0.0", "root"},
		{"pinned_1.0.0", "pinned"},
	} {
		got, err := resolveBody(r, tc.id)
		if err != nil {
			t.Errorf("Resolve(%q) = %v, want %q", tc.id, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
	if _, err := resolveBody(r, "absent_1.0.0"); !errors.Is(err, bmm.ErrSchemaNotFound) {
		t.Errorf("Resolve(absent_1.0.0) = %v, want errors.Is(err, bmm.ErrSchemaNotFound)", err)
	}
}

// TestSchemaResolver_REQ041_RefusesAnIDBothDirectoriesHold — a root with a
// pinned schema's id would take that schema's place for every schema that
// includes it, so the resolver refuses the id rather than pick one.
func TestSchemaResolver_REQ041_RefusesAnIDBothDirectoriesHold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSchemaFiles(t, dir, map[string]string{
		"both_1.0.0.bmm.json":                 "pinned",
		rootsDirName + "/both_1.0.0.bmm.json": "root",
	})
	_, err := resolveBody(newSchemaResolver(dir), "both_1.0.0")
	if !errors.Is(err, errRootTakesPinnedID) {
		t.Fatalf("Resolve(both_1.0.0) = %v, want errors.Is(err, errRootTakesPinnedID)", err)
	}
}

// TestSchemaResolver_REQ041_ReadsAResourcesDirectoryWithoutRoots — a resources
// directory with no roots directory, such as a test's scratch copy of the
// pinned schemas, resolves every id among the pinned schemas.
func TestSchemaResolver_REQ041_ReadsAResourcesDirectoryWithoutRoots(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSchemaFiles(t, dir, map[string]string{"pinned_1.0.0.bmm.json": "pinned"})
	if got, err := resolveBody(newSchemaResolver(dir), "pinned_1.0.0"); err != nil || got != "pinned" {
		t.Errorf("Resolve(pinned_1.0.0) = %q, %v; want %q, nil", got, err, "pinned")
	}
}

// TestBuildPlan_REQ041_SourceLabelNamesTheRootAndItsIncludes — the generated
// header names the file each schema of the plan came from, the root first:
// the RM target's root under sdk/ and the pinned schemas it includes, and the
// AOM 1.4 target's pinned schema and its include.
func TestBuildPlan_REQ041_SourceLabelNamesTheRootAndItsIncludes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		target Target
		want   string
	}{
		{TargetRM, "sdk/openehr_reference_model_1.2.0.bmm.json + openehr_rm_1.2.0.bmm.json + openehr_base_1.3.0.bmm.json"},
		{TargetAOM14, "openehr_am_1.4.0.bmm.json + openehr_base_1.3.0.bmm.json"},
	} {
		plan, err := BuildPlanForTarget(context.Background(), tc.target, testResolver())
		if err != nil {
			t.Fatalf("BuildPlanForTarget(%s): %v", tc.target.GoPackage, err)
		}
		if plan.SourceLabel != tc.want {
			t.Errorf("BuildPlanForTarget(%s).SourceLabel = %q, want %q", tc.target.GoPackage, plan.SourceLabel, tc.want)
		}
		if got := renderGeneratedHeader(plan); !containsLine(got, "// Source: "+tc.want) {
			t.Errorf("renderGeneratedHeader(%s) = %q, want a line %q", tc.target.GoPackage, got, "// Source: "+tc.want)
		}
	}
}

// TestBuildPlan_REQ041_ReadsEachSchemaOnce — the merged schema and the SDK
// extension keys come from one read of each schema, so a schema that changes
// between two reads cannot put two versions of itself into one plan.
func TestBuildPlan_REQ041_ReadsEachSchemaOnce(t *testing.T) {
	t.Parallel()
	for _, target := range []Target{TargetRM, TargetAOM14} {
		r := &countingResolver{Resolver: testResolver(), calls: map[string]int{}}
		if _, err := BuildPlanForTarget(t.Context(), target, r); err != nil {
			t.Fatalf("BuildPlanForTarget(%s): %v", target.GoPackage, err)
		}
		if r.calls[target.RootID] == 0 {
			t.Errorf("BuildPlanForTarget(%s) never resolved its root %s", target.GoPackage, target.RootID)
		}
		for id, n := range r.calls {
			if n != 1 {
				t.Errorf("BuildPlanForTarget(%s) resolved %s %d times, want once", target.GoPackage, id, n)
			}
		}
	}
}

// countingResolver counts the times each schema id is resolved.
type countingResolver struct {
	bmm.Resolver
	calls map[string]int
}

func (r *countingResolver) Resolve(ctx context.Context, id string) (io.ReadCloser, error) {
	r.calls[id]++
	return r.Resolver.Resolve(ctx, id)
}

// containsLine reports whether s holds line as one whole line.
func containsLine(s, line string) bool {
	for l := range strings.Lines(s) {
		if strings.TrimSuffix(l, "\n") == line {
			return true
		}
	}
	return false
}

// stageResources copies the vendored schemas, the pinned ones and the roots
// under sdk/, into a scratch resources directory and returns it. edit sees
// each file by its path relative to the resources directory and returns the
// bytes to write in its place.
func stageResources(t *testing.T, edit func(name string, body []byte) []byte) string {
	t.Helper()
	stage := t.TempDir()
	for _, dir := range []string{"", rootsDirName} {
		entries, err := os.ReadDir(filepath.Join(testResources, dir))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(testResources, dir), err)
		}
		if err := os.MkdirAll(filepath.Join(stage, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := path.Join(dir, e.Name())
			body, err := os.ReadFile(filepath.Join(testResources, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(name)), edit(name, body), 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
	return stage
}
