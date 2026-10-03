package config

import (
	"fmt"
	"reflect"

	"github.com/spf13/pflag"
)

const (
	StartupLogAll                = "all"
	StartupLogRedactConfidential = "redact-confidential"
	StartupLogNone               = "none"
)

func ConfigFields(mode string, fs pflag.FlagSet, cfg any) (map[string]any, error) {
	switch mode {
	case StartupLogNone:
		return nil, nil
	case StartupLogAll, StartupLogRedactConfidential:
	default:
		return nil, fmt.Errorf("invalid log startup mode %q, must be one of [%s, %s, %s]",
			mode, StartupLogAll, StartupLogRedactConfidential, StartupLogNone)
	}

	confidential := map[uintptr]bool{}
	if mode == StartupLogRedactConfidential {
		collectConfidential(reflect.ValueOf(cfg).Elem(), confidential)
	}

	flagToValue := make(map[string]any)
	fs.VisitAll(func(f *pflag.Flag) {
		if confidential[destAddr(f.Value)] {
			flagToValue[f.Name] = "redacted"
			return
		}
		flagToValue[f.Name] = f.Value.String()
	})
	return flagToValue, nil
}

func collectConfidential(v reflect.Value, out map[uintptr]bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fv := v.Field(i)
		ft := t.Field(i)

		switch {
		case fv.Kind() == reflect.Struct:
			collectConfidential(fv, out)
		case fv.Kind() == reflect.Ptr && ft.Type.Elem().Kind() == reflect.Struct:
			if !fv.IsNil() {
				collectConfidential(fv.Elem(), out)
			}
		case ft.Tag.Get("confidential") == "true":
			out[fv.UnsafeAddr()] = true
		}
	}
}

func destAddr(v pflag.Value) uintptr {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return 0
	}
	return rv.Pointer()
}
