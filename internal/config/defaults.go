package config

import "reflect"

func applyDefaults(cfg *Config) error {
	return applyDefaultsValue(reflect.ValueOf(cfg).Elem())
}

func applyDefaultsValue(v reflect.Value) error {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		fieldValue := v.Field(i)
		fieldType := t.Field(i)

		if fieldValue.Kind() == reflect.Struct {
			if err := applyDefaultsValue(fieldValue); err != nil {
				return err
			}

			continue
		}

		raw, ok := fieldType.Tag.Lookup("default")

		if !ok || !fieldValue.IsZero() {
			continue
		}

		if err := setTaggedValue(fieldValue, "default for "+fieldType.Name, raw); err != nil {
			return err
		}
	}

	return nil
}
