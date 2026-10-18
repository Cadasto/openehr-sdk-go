package discovery

import "slices"

// Clone returns a copy of a that shares nothing with a, so a change to one
// does not reach the other. Each endpoint URL in the copy is a new value with
// its own copy of the user information, and each list is a new slice. A nil
// URL or list stays nil, and the flag is copied as it is.
func (a AuthEndpoints) Clone() AuthEndpoints {
	for _, m := range authMembers {
		u := m.endpoint(&a)
		if *u == nil {
			continue
		}
		c := **u
		if c.User != nil {
			user := *c.User
			c.User = &user
		}
		*u = &c
	}
	for _, l := range []*[]string{
		&a.ScopesSupported, &a.ResponseTypesSupported, &a.CodeChallengeMethodsSupported,
		&a.GrantTypesSupported, &a.TokenEndpointAuthMethodsSupported,
		&a.TokenEndpointAuthSigningAlgValuesSupported, &a.IDTokenSigningAlgValuesSupported,
		&a.Capabilities,
	} {
		*l = slices.Clone(*l)
	}
	return a
}
