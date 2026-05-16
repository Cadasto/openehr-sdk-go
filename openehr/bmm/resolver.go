package bmm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Resolver hands the loader the bytes for a given schema id. The id is
// the canonical form used inside the BMM "includes" map, e.g.
// "openehr_base_1.3.0". Implementations are stateless from the
// loader's perspective; they may consult network, disk, or memory.
//
// On miss, implementations MUST return an error satisfying
// errors.Is(err, ErrSchemaNotFound).
type Resolver interface {
	Resolve(ctx context.Context, schemaID string) (io.ReadCloser, error)
}

// FSResolver reads "<id>.bmm.json" files from a base directory.
// Typical usage: FSResolver{Root: "resources/bmm"}.
type FSResolver struct {
	Root string
}

// Resolve implements Resolver. Joins Root + schemaID + ".bmm.json".
// Returns ErrSchemaNotFound (wrapped) on missing files.
func (r FSResolver) Resolve(_ context.Context, schemaID string) (io.ReadCloser, error) {
	if schemaID == "" {
		return nil, fmt.Errorf("%w: empty schema id", ErrSchemaNotFound)
	}
	path := filepath.Join(r.Root, schemaID+".bmm.json")
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q (looked at %s)", ErrSchemaNotFound, schemaID, path)
		}
		return nil, fmt.Errorf("bmm.FSResolver.Resolve %q: %w", schemaID, err)
	}
	return f, nil
}

// MapResolver resolves from an in-memory map of id → JSON bytes. Useful
// for tests and for embedding BMM bytes into a binary.
type MapResolver map[string][]byte

// Resolve implements Resolver.
func (m MapResolver) Resolve(_ context.Context, schemaID string) (io.ReadCloser, error) {
	if schemaID == "" {
		return nil, fmt.Errorf("%w: empty schema id", ErrSchemaNotFound)
	}
	b, ok := m[schemaID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrSchemaNotFound, schemaID)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
