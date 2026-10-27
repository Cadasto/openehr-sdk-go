package smart

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"maps"
)

// PrincipalType is a platform-issued principal category.
type PrincipalType string

// The platform-issued principal categories carried on the ID token.
const (
	PrincipalTypePerson  PrincipalType = "PERSON"
	PrincipalTypeAgent   PrincipalType = "AGENT"
	PrincipalTypeUnknown PrincipalType = ""
)

// PrincipalIdentity carries tenant-scoped principal claims when the
// deployment issues them on the ID token.
//
// Printed with fmt, a PrincipalIdentity shows the tokens its Raw holds as
// "[redacted]": see [PrincipalIdentity.String]. Logged with log/slog, a
// *PrincipalIdentity shows them the same way: see
// [PrincipalIdentity.LogValue].
type PrincipalIdentity struct {
	UID  string
	Type PrincipalType
	// Raw is a defensive shallow copy of the claims map from which this
	// identity was extracted. Callers may read it freely; mutations to the
	// original claims map after construction do not affect Raw (and vice
	// versa). Nested values are not deep-copied. When the claims come from
	// the token-endpoint body, Raw holds its members verbatim, the access
	// and refresh tokens included, so it must not be printed or logged as it
	// is; print or log the *PrincipalIdentity itself, whose String and
	// LogValue mask them.
	Raw map[string]any
}

// String describes p with Raw shown as a copy whose access_token,
// refresh_token and id_token members keep their keys but have their values
// replaced by "[redacted]". UID and Type are shown as they are; p and its Raw
// are not changed. A PrincipalIdentity value copied out of a
// *PrincipalIdentity prints the same way.
func (p PrincipalIdentity) String() string {
	return fmt.Sprintf("smart.PrincipalIdentity{UID: %q, Type: %q, Raw: %v}", p.UID, p.Type, maskedTokens(p.Raw))
}

// GoString returns the text of [PrincipalIdentity.String], so that printing p
// in Go syntax shows no token either.
func (p PrincipalIdentity) GoString() string { return p.String() }

// Format writes the text of [PrincipalIdentity.String] for every fmt verb it
// is called for, whatever the flags and width. fmt answers %T and %p without
// calling it, and %p of a PrincipalIdentity value prints its fields, so never
// print a PrincipalIdentity with %p.
func (p PrincipalIdentity) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, p.String())
}

// LogValue lets log/slog log p without the tokens Raw can hold. It logs UID,
// Type and a copy of Raw as a group, keyed by field name; in the copy the
// access_token, refresh_token and id_token members keep their keys but have
// their values replaced by "[redacted]". p itself is not changed. A nil p
// logs as nil, as slog logs any nil pointer.
//
// The masking applies to a *PrincipalIdentity, such as the one
// [LaunchContext].Principal holds. A PrincipalIdentity value copied out of
// it (*p) and logged as it is does not go through LogValue: slog's text
// handler prints it with the text of [PrincipalIdentity.String], which masks
// the same members, but its JSON handler encodes its fields, tokens included.
// Log the pointer instead.
func (p *PrincipalIdentity) LogValue() slog.Value {
	if p == nil {
		return slog.AnyValue(nil)
	}
	return slog.GroupValue(
		slog.String("UID", p.UID),
		slog.String("Type", string(p.Type)),
		slog.Any("Raw", maskedTokens(p.Raw)),
	)
}

// PrincipalClaimNames configures claim keys for [PrincipalIdentity]
// extraction. Zero values use the Cadasto defaults.
type PrincipalClaimNames struct {
	UIDClaim  string // default principal_uid
	TypeClaim string // default principal_type
}

func principalFromClaims(claims map[string]any, names PrincipalClaimNames) *PrincipalIdentity {
	// A claim that is absent or not a string reads as "".
	uid, _ := claims[cmp.Or(names.UIDClaim, "principal_uid")].(string)
	typRaw, _ := claims[cmp.Or(names.TypeClaim, "principal_type")].(string)
	if uid == "" && typRaw == "" {
		return nil
	}
	pt := PrincipalType(typRaw)
	if typRaw != "" && pt != PrincipalTypePerson && pt != PrincipalTypeAgent {
		pt = PrincipalTypeUnknown
	}
	return &PrincipalIdentity{UID: uid, Type: pt, Raw: maps.Clone(claims)}
}
