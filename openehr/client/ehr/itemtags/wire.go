package itemtags

import openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"

// Tag is an ITEM_TAG wire value. Alias of [openehrclient.ItemTag].
type Tag = openehrclient.ItemTag

// FormatHeader encodes tags into the ITS-REST header shape.
func FormatHeader(tags []Tag) (string, error) {
	return openehrclient.FormatItemTagHeader(tags)
}

// ParseHeader decodes an ITS-REST item-tag header value.
func ParseHeader(header string) ([]Tag, error) {
	return openehrclient.ParseItemTagHeader(header)
}
