package config

import (
	"fmt"
	"reflect"
	"strconv"
)

// THIS FILE IS VIBECODED, only file because why would I touch reflection in Go... I just want print

const (
	StartupLogAll                = "all"
	StartupLogRedactConfidential = "redact-confidential"
	StartupLogNone               = "none"
)

var stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

// ConfigFields converts a config struct (or pointer to one) into a nested map
// suitable for structured logging.
//
// mode:
//   - "all":                 include everything, including confidential fields
//   - "redact-confidential": omit fields tagged `confidential:"true"`
//   - "none":                return a nil map
func ConfigFields(cfg any, mode string) (map[string]any, error) {
	switch mode {
	case StartupLogNone:
		return nil, nil
	case StartupLogAll, StartupLogRedactConfidential:
	default:
		return nil, fmt.Errorf("invalid log startup mode %q, must be one of [%s, %s, %s]",
			mode, StartupLogAll, StartupLogRedactConfidential, StartupLogNone)
	}

	v := reflect.ValueOf(cfg)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return map[string]any{}, nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("ConfigFields expects a struct or pointer to struct, got %s", v.Kind())
	}

	c := &collector{
		rootPkg: v.Type().PkgPath(),
		omit:    mode == StartupLogRedactConfidential,
		seen:    map[uintptr]bool{},
	}
	return c.structFields(v), nil
}

type collector struct {
	rootPkg string
	omit    bool
	seen    map[uintptr]bool // pointer cycle protection
}

func isConfidential(f reflect.StructField) bool {
	b, err := strconv.ParseBool(f.Tag.Get("confidential"))
	return err == nil && b
}

func (c *collector) structFields(v reflect.Value) map[string]any {
	t := v.Type()
	out := make(map[string]any, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if c.omit && isConfidential(f) {
			continue
		}
		out[f.Name] = c.value(v.Field(i))
	}
	return out
}

func (c *collector) value(v reflect.Value) any {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		if v.Kind() == reflect.Pointer {
			ptr := v.Pointer()
			if c.seen[ptr] {
				return "<cycle>"
			}
			c.seen[ptr] = true
			defer delete(c.seen, ptr)
		}
		v = v.Elem()
	}

	// Self-describing types (time.Duration, time.Time, ...) become strings so
	// JSON output stays readable (a Duration would otherwise be raw nanoseconds).
	if v.Type().Implements(stringerType) && v.CanInterface() {
		return fmt.Sprint(v.Interface())
	}

	if v.Kind() == reflect.Struct {
		// Foreign structs (e.g. tls.Config) can hold secrets, funcs and mutexes.
		if v.Type().PkgPath() != c.rootPkg {
			return "<" + v.Type().String() + ">"
		}
		return c.structFields(v)
	}

	if v.CanInterface() {
		return v.Interface()
	}
	return nil
}

// Flatten turns a nested map into a single-level map with dotted keys, e.g.
// {"A": {"B": 1}} becomes {"A.B": 1}.
func Flatten(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	flattenInto(out, "", m)
	return out
}

func flattenInto(out map[string]any, prefix string, m map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flattenInto(out, key, sub)
			continue
		}
		out[key] = v
	}
}
