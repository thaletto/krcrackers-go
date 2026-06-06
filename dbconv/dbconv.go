// Package dbconv provides forgiving coercions from generic `any` values
// to Go primitive types. It is intended for the kind of polymorphic
// results produced by the database.DB interface (and JSON-decoded D1
// responses): the same column can be reported as int64, float64, string,
// or []byte depending on the driver, and D1 may return numeric columns
// as JSON numbers (float64) where SQLite would give int64.
//
// Functions in this package never panic and never return an error: an
// unexpected or nil value yields the type's zero value. This matches the
// behavior callers in this repo relied on before the helpers were
// extracted from the products service.
package dbconv

import (
	"fmt"
	"strconv"
)

// Int coerces v to int. Returns 0 for nil or unrecognised types.
func Int(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// Float coerces v to float64. Returns 0 for nil or unrecognised types.
func Float(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// String coerces v to string. Returns "" for nil. Falls back to
// fmt.Sprint for unrecognised types.
func String(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// NullableString coerces v to *string. Returns nil for nil v. Non-nil
// results are always non-nil pointers to a non-empty (or empty) string.
func NullableString(v any) *string {
	if v == nil {
		return nil
	}
	s := String(v)
	return &s
}
