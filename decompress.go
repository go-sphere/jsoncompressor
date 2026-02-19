package jsoncompressor

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
)

func Unmarshal(data []byte, target any) error {
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}
	var jsonData any
	err := json.Unmarshal(data, &jsonData)
	if err != nil {
		return err
	}
	return decodeIntoValue(jsonData, val.Elem())
}

func decodeIntoStruct(data []any, val reflect.Value) error {
	meta := getStructMeta(val.Type())
	if meta == nil {
		if len(data) != 0 {
			return fmt.Errorf("field count mismatch: have %d values, want %d", len(data), 0)
		}
		return nil
	}
	if len(data) != len(meta.fields) {
		return fmt.Errorf("field count mismatch: have %d values, want %d", len(data), len(meta.fields))
	}
	for i, f := range meta.fields {
		field := val.Field(f.index)
		if err := decodeIntoValue(data[i], field); err != nil {
			return err
		}
	}
	return nil
}

func decodeIntoMap(data map[string]any, field reflect.Value) error {
	if field.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("unsupported map key type: %s", field.Type().Key())
	}
	if !field.CanSet() {
		return fmt.Errorf("cannot set value of type %s", field.Type())
	}
	if field.IsNil() {
		field.Set(reflect.MakeMapWithSize(field.Type(), len(data)))
	}
	for key, raw := range data {
		value := reflect.New(field.Type().Elem()).Elem()
		if err := decodeIntoValue(raw, value); err != nil {
			return err
		}
		field.SetMapIndex(reflect.ValueOf(key).Convert(field.Type().Key()), value)
	}
	return nil
}

func decodeIntoValue(data any, field reflect.Value) error {
	if field.Kind() == reflect.Pointer {
		if data == nil {
			if field.CanSet() {
				field.Set(reflect.Zero(field.Type()))
			}
			return nil
		}
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return decodeIntoValue(data, field.Elem())
	}
	if data == nil {
		if field.CanSet() {
			field.Set(reflect.Zero(field.Type()))
		}
		return nil
	}
	switch field.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Uintptr:
		return fmt.Errorf("unsupported kind: %s", field.Kind())
	case reflect.Map:
		dataMap, ok := data.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object for map field")
		}
		return decodeIntoMap(dataMap, field)
	case reflect.Struct:
		dataSlice, ok := data.([]any)
		if !ok {
			return fmt.Errorf("expected array for struct field")
		}
		return decodeIntoStruct(dataSlice, field)
	case reflect.Slice:
		dataSlice, ok := data.([]any)
		if !ok {
			return fmt.Errorf("expected array for slice field")
		}
		slice := reflect.MakeSlice(field.Type(), len(dataSlice), len(dataSlice))
		for i := range dataSlice {
			if err := decodeIntoValue(dataSlice[i], slice.Index(i)); err != nil {
				return err
			}
		}
		if field.CanSet() {
			field.Set(slice)
		}
		return nil
	case reflect.Array:
		dataSlice, ok := data.([]any)
		if !ok {
			return fmt.Errorf("expected array for array field")
		}
		if len(dataSlice) != field.Len() {
			return fmt.Errorf("array length mismatch: have %d values, want %d", len(dataSlice), field.Len())
		}
		for i := range dataSlice {
			if err := decodeIntoValue(dataSlice[i], field.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Interface:
		if field.CanSet() {
			field.Set(reflect.ValueOf(data))
		}
		return nil
	default:
		return assignScalar(field, data)
	}
}

func assignScalar(field reflect.Value, data any) error {
	if !field.CanSet() {
		return fmt.Errorf("cannot set value of type %s", field.Type())
	}
	switch field.Kind() {
	case reflect.String:
		value, ok := data.(string)
		if !ok {
			return fmt.Errorf("expected string for %s", field.Type())
		}
		field.SetString(value)
		return nil
	case reflect.Bool:
		value, ok := data.(bool)
		if !ok {
			return fmt.Errorf("expected bool for %s", field.Type())
		}
		field.SetBool(value)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, ok := data.(float64)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		if math.Trunc(value) != value {
			return fmt.Errorf("expected integer for %s", field.Type())
		}
		intValue := int64(value)
		if field.OverflowInt(intValue) {
			return fmt.Errorf("integer overflow for %s", field.Type())
		}
		field.SetInt(intValue)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, ok := data.(float64)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		if value < 0 || math.Trunc(value) != value {
			return fmt.Errorf("expected unsigned integer for %s", field.Type())
		}
		uintValue := uint64(value)
		if field.OverflowUint(uintValue) {
			return fmt.Errorf("unsigned integer overflow for %s", field.Type())
		}
		field.SetUint(uintValue)
		return nil
	case reflect.Float32, reflect.Float64:
		value, ok := data.(float64)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		if field.OverflowFloat(value) {
			return fmt.Errorf("float overflow for %s", field.Type())
		}
		field.SetFloat(value)
		return nil
	default:
		value := reflect.ValueOf(data)
		if value.Type().AssignableTo(field.Type()) {
			field.Set(value)
			return nil
		}
		if value.Type().ConvertibleTo(field.Type()) {
			field.Set(value.Convert(field.Type()))
			return nil
		}
		return fmt.Errorf("unsupported assignment: %T to %s", data, field.Type())
	}
}
