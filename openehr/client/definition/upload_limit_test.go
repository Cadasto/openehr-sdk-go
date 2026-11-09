package definition_test

// upload_limit_test.go — REQ-108 § Definition template upload: the size
// limit a caller can check a body against before it calls UploadTemplate.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
)

func TestMaxUploadBytesIs32MiB(t *testing.T) { // REQ-108
	if got, want := int64(definition.MaxUploadBytes), int64(32<<20); got != want {
		t.Errorf("definition.MaxUploadBytes = %d, want %d (32 MiB)", got, want)
	}
}
