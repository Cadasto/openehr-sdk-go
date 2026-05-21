package ehr

import (
	"fmt"
	"strings"
)

// ItemTag is a parsed ITEM_TAG from an openehr-item-tag or
// openehr-version-item-tag header value (REQ-059).
type ItemTag struct {
	Key        string
	Value      string
	TargetPath string
}

// FormatItemTagHeader encodes tags into the semicolon-separated ITS-REST
// header shape.
func FormatItemTagHeader(tags []ItemTag) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(tags))
	for i, t := range tags {
		if strings.TrimSpace(t.Key) == "" {
			return "", fmt.Errorf("ehr: item tag[%d]: empty key", i)
		}
		var b strings.Builder
		b.WriteString(`key="`)
		b.WriteString(escapeItemTagValue(t.Key))
		b.WriteByte('"')
		if t.Value != "" {
			b.WriteString(`,value="`)
			b.WriteString(escapeItemTagValue(t.Value))
			b.WriteByte('"')
		}
		if t.TargetPath != "" {
			b.WriteString(`,target_path="`)
			b.WriteString(escapeItemTagValue(t.TargetPath))
			b.WriteByte('"')
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "; "), nil
}

// ParseItemTagHeader decodes the semicolon-separated ITS-REST header
// value. Returns nil, nil for an empty header.
func ParseItemTagHeader(header string) ([]ItemTag, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, nil
	}
	segments := strings.Split(header, ";")
	out := make([]ItemTag, 0, len(segments))
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		t, err := parseItemTagSegment(seg)
		if err != nil {
			return nil, fmt.Errorf("ehr: item tag segment[%d]: %w", i, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func parseItemTagSegment(seg string) (ItemTag, error) {
	var t ItemTag
	for seg != "" {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			break
		}
		key, val, rest, err := readItemTagPair(seg)
		if err != nil {
			return ItemTag{}, err
		}
		switch key {
		case "key":
			t.Key = val
		case "value":
			t.Value = val
		case "target_path":
			t.TargetPath = val
		default:
			return ItemTag{}, fmt.Errorf("unknown field %q", key)
		}
		seg = rest
		if seg != "" {
			if !strings.HasPrefix(seg, ",") {
				return ItemTag{}, fmt.Errorf("expected comma after %q", key)
			}
			seg = strings.TrimSpace(seg[1:])
		}
	}
	if t.Key == "" {
		return ItemTag{}, fmt.Errorf("missing key")
	}
	return t, nil
}

func readItemTagPair(seg string) (key, val, rest string, err error) {
	eq := strings.Index(seg, "=")
	if eq < 0 {
		return "", "", "", fmt.Errorf("expected key=value")
	}
	key = strings.TrimSpace(seg[:eq])
	valPart := strings.TrimSpace(seg[eq+1:])
	val, rest, err = readQuotedItemTagValue(valPart)
	if err != nil {
		return "", "", "", err
	}
	return key, val, rest, nil
}

func readQuotedItemTagValue(valPart string) (val, rest string, err error) {
	if !strings.HasPrefix(valPart, `"`) {
		return "", "", fmt.Errorf("value must be quoted")
	}
	var b strings.Builder
	i := 1
	for i < len(valPart) {
		if valPart[i] == '\\' && i+1 < len(valPart) {
			b.WriteByte(valPart[i+1])
			i += 2
			continue
		}
		if valPart[i] == '"' {
			return b.String(), strings.TrimSpace(valPart[i+1:]), nil
		}
		b.WriteByte(valPart[i])
		i++
	}
	return "", "", fmt.Errorf("unterminated quoted value")
}

func escapeItemTagValue(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
