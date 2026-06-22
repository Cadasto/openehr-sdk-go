package rm

// REQ-122 — version-control derived helper.
//
// VERSION.is_branch is derived from the version's uid (true iff the
// uid's version_tree_id is a branch, per REQ-120). The abstract
// VERSION<T> carries no uid storage, so the derivation is hand-written
// on the concrete versions that do — ORIGINAL_VERSION (stored uid) and
// IMPORTED_VERSION (uid derived from the wrapped item). The generated
// abstract VERSION.is_branch stub is suppressed (manual_impl.go).
//
// Version *container* management (VERSIONED_OBJECT operations, commit_*)
// is out of scope and stays as fail-loud generated stubs: this SDK's
// versioning is server-mediated over REST, not an in-memory container.
// See docs/specifications/rm-functions.md § REQ-122 and ADR 0011.

// IsBranch reports whether this version represents a branch, derived
// from its uid's version_tree_id. REQ-122.
func (o *OriginalVersion[T]) IsBranch() bool {
	vt := o.UID.VersionTreeID()
	return vt.IsBranch()
}

// IsBranch reports whether this imported version represents a branch,
// derived from the wrapped original version's uid. REQ-122.
func (i *ImportedVersion[T]) IsBranch() bool {
	vt := i.Item.UID.VersionTreeID()
	return vt.IsBranch()
}
