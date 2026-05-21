package auth

import "encoding/json"

// ParseOAuth2Error decodes the RFC 6749 § 5.2 error envelope from a
// token-endpoint response body. Returns nil when the body does not
// match the envelope shape.
func ParseOAuth2Error(body []byte) *OAuth2Error {
	var env struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorURI         string `json:"error_uri"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Error == "" {
		return nil
	}
	return &OAuth2Error{Code: env.Error, Description: env.ErrorDescription, URI: env.ErrorURI}
}
