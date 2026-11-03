package bmmgen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// rootsDirName is the sub-directory of the resources directory that holds the
// root schemas the SDK authors over the pinned openEHR schemas.
const rootsDirName = "sdk"

// schemaFileSuffix is the file name suffix of a schema: [bmm.FSResolver]
// resolves an id as <dir>/<id>.bmm.json.
const schemaFileSuffix = ".bmm.json"

// errRootTakesPinnedID reports a schema id that both the roots directory and
// the pinned schemas hold.
var errRootTakesPinnedID = errors.New("bmmgen: a root schema takes a pinned schema's id")

// schemaResolver resolves a schema id against the root schemas first and the
// pinned schemas second. It refuses an id both hold: a root under a pinned
// schema's id would replace that schema for every schema that includes it.
type schemaResolver struct {
	roots  string
	pinned string
}

// NewResolver returns the resolver [Run] reads a target's schemas through,
// over resourcesDir: the root schemas the SDK authors, in its sdk
// sub-directory, before the pinned schemas at its top. A test that loads the
// schema a target names, such as [TargetRM].RootID, loads it through this
// resolver.
func NewResolver(resourcesDir string) bmm.Resolver {
	return newSchemaResolver(resourcesDir)
}

// newSchemaResolver returns the resolver over resourcesDir: its root schemas
// in the sdk sub-directory, its pinned schemas at the top. A resources
// directory without the sub-directory resolves every id among its pinned
// schemas.
func newSchemaResolver(resourcesDir string) schemaResolver {
	return schemaResolver{roots: filepath.Join(resourcesDir, rootsDirName), pinned: resourcesDir}
}

// Resolve implements [bmm.Resolver].
func (r schemaResolver) Resolve(ctx context.Context, id string) (io.ReadCloser, error) {
	isRoot, err := r.isRoot(id)
	if err != nil {
		return nil, err
	}
	if !isRoot {
		return bmm.FSResolver{Root: r.pinned}.Resolve(ctx, id)
	}
	switch _, err := os.Stat(filepath.Join(r.pinned, id+schemaFileSuffix)); {
	case err == nil:
		return nil, fmt.Errorf("%w: %q is in %s and in %s", errRootTakesPinnedID, id, r.roots, r.pinned)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("bmmgen: resolve %q: %w", id, err)
	}
	return bmm.FSResolver{Root: r.roots}.Resolve(ctx, id)
}

// isRoot reports whether id names a file in the roots directory.
func (r schemaResolver) isRoot(id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	_, err := os.Stat(filepath.Join(r.roots, id+schemaFileSuffix))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("bmmgen: resolve %q: %w", id, err)
	}
}

// schemaFile names the file id resolves to, relative to the resources
// directory, as a generated header prints it.
func (r schemaResolver) schemaFile(id string) (string, error) {
	isRoot, err := r.isRoot(id)
	if err != nil || !isRoot {
		return id + schemaFileSuffix, err
	}
	return rootsDirName + "/" + id + schemaFileSuffix, nil
}

// schemaFiler is a resolver that names the file an id resolves to.
type schemaFiler interface {
	schemaFile(id string) (string, error)
}

// sourceLabel names the files the schemas ids name were read from, joined
// as a generated header prints them after "// Source: ". A resolver that
// does not name its files gets each id's file name.
func sourceLabel(ids []string, resolver bmm.Resolver) (string, error) {
	filer, named := resolver.(schemaFiler)
	files := make([]string, len(ids))
	for i, id := range ids {
		if !named {
			files[i] = id + schemaFileSuffix
			continue
		}
		f, err := filer.schemaFile(id)
		if err != nil {
			return "", err
		}
		files[i] = f
	}
	return strings.Join(files, " + "), nil
}
