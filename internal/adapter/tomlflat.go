package adapter

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// encodeOneLevelTOML serializes a TOML document (decoded into map[string]any)
// keeping at most one level of [table] header: each top-level table becomes a
// single `[section]`, and everything inside a section (including deeper nesting)
// uses TOML 1.0.0 dotted keys, so the output never indents. Root-level scalars
// and arrays are emitted first, then one block per top-level table.
//
// Example:
//
//	model = "m"
//
//	[model_providers]
//	proxy.base_url = "https://…"
//	proxy.env_key = "PROXY_TOKEN"
func encodeOneLevelTOML(m map[string]any) ([]byte, error) {
	var b strings.Builder

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 1) Root-level non-table values (scalars, arrays, empty/inline tables).
	for _, k := range keys {
		if sub, ok := asStringMap(m[k]); ok && len(sub) > 0 {
			continue // non-empty tables become [section] blocks below
		}
		val, err := encodeTOMLValue(m[k])
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", k, err)
		}
		b.WriteString(dottedKey([]string{k}) + " = " + val + "\n")
	}

	// 2) One [section] block per top-level table; contents use dotted keys.
	for _, k := range keys {
		sub, ok := asStringMap(m[k])
		if !ok || len(sub) == 0 {
			continue
		}
		b.WriteString("\n[" + dottedKey([]string{k}) + "]\n")
		if err := writeDotted(&b, nil, sub); err != nil {
			return nil, err
		}
	}

	return []byte(b.String()), nil
}

func writeDotted(b *strings.Builder, prefix []string, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		path := append(append([]string{}, prefix...), k)
		v := m[k]
		if sub, ok := asStringMap(v); ok {
			if len(sub) == 0 {
				b.WriteString(dottedKey(path) + " = {}\n")
				continue
			}
			if err := writeDotted(b, path, sub); err != nil {
				return err
			}
			continue
		}
		val, err := encodeTOMLValue(v)
		if err != nil {
			return fmt.Errorf("encoding %s: %w", strings.Join(path, "."), err)
		}
		b.WriteString(dottedKey(path) + " = " + val + "\n")
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
