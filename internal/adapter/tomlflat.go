package adapter

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// encodeSectionedTOML serializes a TOML document (decoded into map[string]any)
// using standard [table] headers whose names are the dotted path, with plain
// (non-dotted) keys inside each section and no indentation. Intermediate tables
// that contain only sub-tables are collapsed into the dotted header (so a lone
// parent like [model_providers] is not emitted when only [model_providers.proxy]
// has content).
//
// Example:
//
//	model = "m"
//
//	[model_providers.proxy]
//	base_url = "https://…"
//	env_key = "PROXY_TOKEN"
//
//	[model_providers.proxy.env_http_headers]
//	X-Api-Key = "…"
func encodeSectionedTOML(m map[string]any) ([]byte, error) {
	var b strings.Builder
	if err := emitTable(&b, nil, m, true); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// emitTable writes one table: its leaf key/value pairs under a [path] header
// (omitted for the root), then recurses into child tables as their own sections.
// A "leaf" is any value that is not a non-empty table (scalars, arrays, and
// empty inline tables); a "sub-table" is a non-empty map, which becomes a deeper
// [path.child] section.
func emitTable(b *strings.Builder, path []string, m map[string]any, isRoot bool) error {
	var leaves, subs []string
	for k, v := range m {
		if sm, ok := asStringMap(v); ok && len(sm) > 0 {
			subs = append(subs, k)
		} else {
			leaves = append(leaves, k)
		}
	}
	sort.Strings(leaves)
	sort.Strings(subs)

	// Emit this table's own header — but skip it for the root, and for an
	// intermediate table that has only sub-tables (no leaves): that header is
	// redundant because its children carry the full dotted path. An explicitly
	// empty table (no leaves, no subs) still needs a header to exist.
	if !isRoot && (len(leaves) > 0 || len(subs) == 0) {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("[" + dottedKey(path) + "]\n")
	}

	for _, k := range leaves {
		val, err := encodeTOMLValue(m[k])
		if err != nil {
			return fmt.Errorf("encoding %s: %w", strings.Join(append(path, k), "."), err)
		}
		keyRepr := k
		if !isBareKey(k) {
			keyRepr = quoteBasic(k)
		}
		b.WriteString(keyRepr + " = " + val + "\n")
	}

	for _, k := range subs {
		sm, _ := asStringMap(m[k])
		child := append(append([]string{}, path...), k)
		if err := emitTable(b, child, sm, false); err != nil {
			return err
		}
	}
	return nil
}

// dottedKey joins key segments with ".", quoting any segment that isn't a bare
// key (TOML bare keys allow A-Z a-z 0-9 _ -).
func dottedKey(path []string) string {
	parts := make([]string, len(path))
	for i, seg := range path {
		if isBareKey(seg) {
			parts[i] = seg
		} else {
			parts[i] = quoteBasic(seg)
		}
	}
	return strings.Join(parts, ".")
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// asStringMap returns v as a map[string]any if it is a (possibly typed) map with
// string keys.
func asStringMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	out := make(map[string]any, rv.Len())
	for _, mk := range rv.MapKeys() {
		out[mk.String()] = rv.MapIndex(mk).Interface()
	}
	return out, true
}

// encodeTOMLValue renders a scalar, array, or inline table value.
func encodeTOMLValue(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", fmt.Errorf("nil value")
	case string:
		return quoteBasic(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float64:
		return formatFloat(x), nil
	case float32:
		return formatFloat(float64(x)), nil
	case time.Time:
		return x.Format(time.RFC3339), nil
	}

	// Inline table (nested map as an array element or direct value).
	if sm, ok := asStringMap(v); ok {
		return encodeInlineTable(sm)
	}

	// Arrays / slices.
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		elems := make([]string, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			ev, err := encodeTOMLValue(rv.Index(i).Interface())
			if err != nil {
				return "", err
			}
			elems[i] = ev
		}
		return "[" + strings.Join(elems, ", ") + "]", nil
	}

	return "", fmt.Errorf("unsupported value type %T", v)
}

func encodeInlineTable(m map[string]any) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		val, err := encodeTOMLValue(m[k])
		if err != nil {
			return "", err
		}
		key := k
		if !isBareKey(k) {
			key = quoteBasic(k)
		}
		parts = append(parts, key+" = "+val)
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// formatFloat renders a float as a valid TOML float (always with a fractional or
// exponent part).
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnN") { // nN covers inf/nan spellings
		s += ".0"
	}
	return s
}

// quoteBasic renders s as a TOML basic string (double-quoted, escaped).
func quoteBasic(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
