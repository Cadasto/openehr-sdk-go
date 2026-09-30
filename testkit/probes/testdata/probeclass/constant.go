package probeclass

import (
	"net/http"

	"github.com/cadasto/openehr-sdk-go/transport"
)

// Probe907ConstantOnly names only constants of connector packages. A
// constant carries no connection, so the probe reaches no backend.
func Probe907ConstantOnly() (string, int) {
	return transport.DefaultCallerAttributionHeader, http.StatusOK
}
