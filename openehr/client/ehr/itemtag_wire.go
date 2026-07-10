package ehr

import (
	"errors"
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
		if hasCtrlChars(t.Key) {
			return "", fmt.Errorf("ehr: item tag[%d]: key contains control characters", i)
		}
		if t.Value != "" && hasCtrlChars(t.Value) {
			return "", fmt.Errorf("ehr: item tag[%d]: value contains control characters", i)
		}
		if t.TargetPath != "" && hasCtrlChars(t.TargetPath) {
			return "", fmt.Errorf("ehr: item tag[%d]: target_path contains control characters", i)
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
		return ItemTag{}, errors.New("missing key")
	}
	return t, nil
}

func readItemTagPair(seg string) (key, val, rest string, err error) {
	before, after, ok := strings.Cut(seg, "=")
	if !ok {
		return "", "", "", errors.New("expected key=value")
	}
	key = strings.TrimSpace(before)
	valPart := strings.TrimSpace(after)
	val, rest, err = readQuotedItemTagValue(valPart)
	if err != nil {
		return "", "", "", err
	}
	return key, val, rest, nil
}

func readQuotedItemTagValue(valPart string) (val, rest string, err error) {
	if !strings.HasPrefix(valPart, `"`) {
		return "", "", errors.New("value must be quoted")
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
	return "", "", errors.New("unterminated quoted value")
}

func escapeItemTagValue(s string) string {
	// Escape backslash first so the reader (which unescapes \x) round-trips
	// a literal backslash, then escape the quote delimiter.
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// hasCtrlChars reports whether s contains a byte disallowed in an HTTP
// header field value (RFC 9110 §5.5): any C0 control byte other than
// horizontal tab, or DEL (0x7F). CR/LF in particular must never reach a
// header value, where they enable header injection.
func hasCtrlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		if (s[i] < 0x20 && s[i] != '\t') || s[i] == 0x7f {
			return true
		}
	}
	return false
}
