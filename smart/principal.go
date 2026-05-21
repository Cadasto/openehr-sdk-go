package smart

// PrincipalType is a platform-issued principal category (REQ-067).
type PrincipalType string

const (
	PrincipalTypePerson  PrincipalType = "PERSON"
	PrincipalTypeAgent   PrincipalType = "AGENT"
	PrincipalTypeUnknown PrincipalType = ""
)

// PrincipalIdentity carries tenant-scoped principal claims when the
// deployment issues them on the ID token (REQ-067).
type PrincipalIdentity struct {
	UID  string
	Type PrincipalType
	Raw  map[string]any
}

// PrincipalClaimNames configures claim keys for [PrincipalIdentity]
// extraction. Zero values use the Cadasto defaults.
type PrincipalClaimNames struct {
	UIDClaim  string // default principal_uid
	TypeClaim string // default principal_type
}

func (n PrincipalClaimNames) uidClaim() string {
	if n.UIDClaim != "" {
		return n.UIDClaim
	}
	return "principal_uid"
}

func (n PrincipalClaimNames) typeClaim() string {
	if n.TypeClaim != "" {
		return n.TypeClaim
	}
	return "principal_type"
}

func principalFromClaims(claims map[string]any, names PrincipalClaimNames) *PrincipalIdentity {
	uid, _ := claimString(claims, names.uidClaim())
	typRaw, _ := claimString(claims, names.typeClaim())
	if uid == "" && typRaw == "" {
		return nil
	}
	pt := PrincipalType(typRaw)
	if typRaw != "" && pt != PrincipalTypePerson && pt != PrincipalTypeAgent {
		pt = PrincipalTypeUnknown
	}
	return &PrincipalIdentity{UID: uid, Type: pt, Raw: claims}
}

func claimString(claims map[string]any, key string) (string, bool) {
	v, ok := claims[key]
	if !ok {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	default:
		return "", false
	}
}
