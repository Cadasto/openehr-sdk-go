package auth

import jsonv2 "encoding/json/v2"

// ParseOAuth2Error decodes the RFC 6749 §5.2 error envelope from a
// token-endpoint response body. Returns nil when the body does not
// match the envelope shape. Member names are matched exactly, and a body
// that repeats one does not match.
func ParseOAuth2Error(body []byte) *OAuth2Error {
	// RFC 6749 §5.2 spells each member name, so names are read exactly and
	// a repeated name refuses the body: an "ERROR" member that follows the
	// real error never stands in for the code that decides whether the
	// failure is worth retrying.
	var env struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorURI         string `json:"error_uri"`
	}
	if err := jsonv2.Unmarshal(body, &env); err != nil || env.Error == "" {
		return nil
	}
	return &OAuth2Error{Code: env.Error, Description: env.ErrorDescription, URI: env.ErrorURI}
}
