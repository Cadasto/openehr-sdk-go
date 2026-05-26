package rmwrite

import "errors"

// ErrUnknownRMType signals that the requested RM class name is not
// in the openehr/rm/typereg registry.
var ErrUnknownRMType = errors.New("rmwrite: unknown RM type")

// ErrUnknownAttribute signals that the (parentType, attrName) pair
// is not addressable on the given parent — either the parent's Go
// concrete type is not enumerated in the closed dispatch, or the
// attribute name is unknown on that type.
var ErrUnknownAttribute = errors.New("rmwrite: unknown attribute on parent")

// ErrTypeMismatch signals that the supplied child value does not
// satisfy the Go type of the target attribute (e.g. assigning a
// *rm.DVText where the slot expects a *rm.Cluster).
var ErrTypeMismatch = errors.New("rmwrite: child type does not match attribute slot")
