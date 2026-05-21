package itemtags

import "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"

// Tag is an ITEM_TAG wire value (REQ-059). Alias of [ehr.ItemTag].
type Tag = ehr.ItemTag

// FormatHeader encodes tags into the ITS-REST header shape.
func FormatHeader(tags []Tag) (string, error) {
	return ehr.FormatItemTagHeader(tags)
}

// ParseHeader decodes an ITS-REST item-tag header value.
func ParseHeader(header string) ([]Tag, error) {
	return ehr.ParseItemTagHeader(header)
}
