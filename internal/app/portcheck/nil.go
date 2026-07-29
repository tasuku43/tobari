// Package portcheck contains the fail-closed nil check shared by application
// ports. An interface can be non-nil while holding a nil pointer, so a direct
// interface comparison is insufficient at an external-call boundary.
package portcheck

import "reflect"

// IsNil reports whether value is nil or contains a typed nil value.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
