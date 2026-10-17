package smart

import (
	"cmp"
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
type PrincipalIdentity struct {
	UID  string
	Type PrincipalType
	// Raw is a defensive shallow copy of the claims map from which this
	// identity was extracted. Callers may read it freely; mutations to the
	// original claims map after construction do not affect Raw (and vice
	// versa). Nested values are not deep-copied. When the claims come from
	// the token-endpoint body, Raw holds its members verbatim, the access
	// and refresh tokens included, so it must not be printed or logged as it
	// is; log the *PrincipalIdentity itself, whose LogValue masks them.
	Raw map[string]any
}

// LogValue lets log/slog log p without the tokens Raw can hold. It logs UID,
// Type and a copy of Raw as a group, keyed by field name; in the copy the
// access_token, refresh_token and id_token members keep their keys but have
// their values replaced by the same fixed marker. p itself is not changed. A
// nil p logs as nil, as slog logs any nil pointer.
//
// The masking applies to a *PrincipalIdentity, such as the one
// [LaunchContext].Principal holds. A PrincipalIdentity value copied out of
// it (*p) and logged as it is does not go through LogValue, so the tokens in
// its Raw are logged: log the pointer instead.
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
