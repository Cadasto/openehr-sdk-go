package itemtags

import (
	"fmt"
	"strings"
)

// Tag is a parsed ITEM_TAG from an openehr-item-tag or
// openehr-version-item-tag header value.
type Tag struct {
	Key        string
	Value      string
	TargetPath string
}

// FormatHeader encodes tags into the semicolon-separated ITS-REST header
// shape (REQ-059).
func FormatHeader(tags []Tag) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(tags))
	for i, t := range tags {
		if strings.TrimSpace(t.Key) == "" {
			return "", fmt.Errorf("itemtags: tag[%d]: empty key", i)
		}
		var b strings.Builder
		b.WriteString(`key="`)
		b.WriteString(escapeHeaderValue(t.Key))
		b.WriteByte('"')
		if t.Value != "" {
			b.WriteString(`,value="`)
			b.WriteString(escapeHeaderValue(t.Value))
			b.WriteByte('"')
		}
		if t.TargetPath != "" {
			b.WriteString(`,target_path="`)
			b.WriteString(escapeHeaderValue(t.TargetPath))
			b.WriteByte('"')
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "; "), nil
}

// ParseHeader decodes the semicolon-separated ITS-REST header value.
// Returns nil, nil for an empty header.
func ParseHeader(header string) ([]Tag, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, nil
	}
	segments := strings.Split(header, ";")
	out := make([]Tag, 0, len(segments))
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		t, err := parseTagSegment(seg)
		if err != nil {
			return nil, fmt.Errorf("itemtags: segment[%d]: %w", i, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func parseTagSegment(seg string) (Tag, error) {
	var t Tag
	for seg != "" {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			break
		}
		key, val, rest, err := readPair(seg)
		if err != nil {
			return Tag{}, err
		}
		switch key {
		case "key":
			t.Key = val
		case "value":
			t.Value = val
		case "target_path":
			t.TargetPath = val
		default:
			return Tag{}, fmt.Errorf("unknown field %q", key)
		}
		seg = rest
		if seg != "" {
			if !strings.HasPrefix(seg, ",") {
				return Tag{}, fmt.Errorf("expected comma after %q", key)
			}
			seg = strings.TrimSpace(seg[1:])
		}
	}
	if t.Key == "" {
		return Tag{}, fmt.Errorf("missing key")
	}
	return t, nil
}

func readPair(seg string) (key, val, rest string, err error) {
	eq := strings.Index(seg, "=")
	if eq < 0 {
		return "", "", "", fmt.Errorf("expected key=value")
	}
	key = strings.TrimSpace(seg[:eq])
	valPart := strings.TrimSpace(seg[eq+1:])
	if !strings.HasPrefix(valPart, `"`) {
		return "", "", "", fmt.Errorf("value must be quoted")
	}
	valPart = valPart[1:]
	end := strings.Index(valPart, `"`)
	if end < 0 {
		return "", "", "", fmt.Errorf("unterminated quoted value")
	}
	val = valPart[:end]
	rest = strings.TrimSpace(valPart[end+1:])
	return key, val, rest, nil
}

func escapeHeaderValue(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
