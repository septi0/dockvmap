package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

func applyEnvOverrides(cfg *Config) error {
	return applyEnvOverridesValue(reflect.ValueOf(cfg).Elem())
}

func applyEnvOverridesValue(v reflect.Value) error {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		fieldValue := v.Field(i)
		fieldType := t.Field(i)

		if fieldValue.Kind() == reflect.Struct {
			if err := applyEnvOverridesValue(fieldValue); err != nil {
				return err
			}

			continue
		}

		name, ok := fieldType.Tag.Lookup("env")

		if !ok {
			continue
		}

		raw, ok := os.LookupEnv(name)

		if !ok {
			continue
		}

		if err := setEnvValue(fieldValue, name, raw); err != nil {
			return err
		}
	}

	return nil
}

func setEnvValue(fieldValue reflect.Value, name, raw string) error {
	switch {
	case fieldValue.Kind() == reflect.String:
		fieldValue.SetString(raw)

	case fieldValue.Kind() == reflect.Int:
		parsed, err := strconv.Atoi(raw)

		if err != nil {
			return fmt.Errorf("invalid %s %q: %w", name, raw, err)
		}

		fieldValue.SetInt(int64(parsed))

	case fieldValue.Kind() == reflect.Bool:
		parsed, err := strconv.ParseBool(raw)

		if err != nil {
			return fmt.Errorf("invalid %s %q: %w", name, raw, err)
		}

		fieldValue.SetBool(parsed)

	case fieldValue.Kind() == reflect.Ptr && fieldValue.Type().Elem().Kind() == reflect.Bool:
		parsed, err := strconv.ParseBool(raw)

		if err != nil {
			return fmt.Errorf("invalid %s %q: %w", name, raw, err)
		}

		fieldValue.Set(reflect.ValueOf(&parsed))

	case fieldValue.Kind() == reflect.Slice && fieldValue.Type().Elem().Kind() == reflect.String:
		fieldValue.Set(reflect.ValueOf(splitEnvList(raw)))

	default:
		return fmt.Errorf("env override: unsupported field type for %s", name)
	}

	return nil
}

func splitEnvList(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")

	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}

	return parts
}
