package jsoncompressor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
)

func Unmarshal(data []byte, target any) error {
	val := reflect.ValueOf(target)
	if val.Kind() != reflect.Pointer || val.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}
	// UseNumber keeps numbers as their literal text so integers above 2^53
	// are not rounded through float64 before reaching assignScalar.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var jsonData any
	if err := dec.Decode(&jsonData); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("invalid data after top-level value")
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
	if field.CanAddr() && ownsJSON(field.Type()) {
		// Types such as time.Time own their JSON form; hand the value back to
		// their UnmarshalJSON instead of decoding it field by field.
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return field.Addr().Interface().(json.Unmarshaler).UnmarshalJSON(raw)
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
			field.Set(reflect.ValueOf(numbersToFloat(data)))
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
		number, ok := data.(json.Number)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		intValue, err := parseInt(number)
		if err != nil {
			return fmt.Errorf("%w for %s", err, field.Type())
		}
		if field.OverflowInt(intValue) {
			return fmt.Errorf("integer overflow for %s", field.Type())
		}
		field.SetInt(intValue)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number, ok := data.(json.Number)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		uintValue, err := parseUint(number)
		if err != nil {
			return fmt.Errorf("%w for %s", err, field.Type())
		}
		if field.OverflowUint(uintValue) {
			return fmt.Errorf("unsigned integer overflow for %s", field.Type())
		}
		field.SetUint(uintValue)
		return nil
	case reflect.Float32, reflect.Float64:
		number, ok := data.(json.Number)
		if !ok {
			return fmt.Errorf("expected number for %s", field.Type())
		}
		value, err := number.Float64()
		if err != nil {
			return fmt.Errorf("float overflow for %s", field.Type())
		}
		if field.OverflowFloat(value) {
			return fmt.Errorf("float overflow for %s", field.Type())
		}
		field.SetFloat(value)
		return nil
	default:
		value := reflect.ValueOf(numbersToFloat(data))
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

// maxExactFloat is the largest magnitude below which every integer is exactly
// representable as a float64 (2^53).
const maxExactFloat = 1 << 53

var (
	errNotInteger         = errors.New("expected integer")
	errNotUnsignedInteger = errors.New("expected unsigned integer")
	errIntegerOverflow    = errors.New("integer overflow")
)

// parseInt converts a JSON number to int64 without going through float64.
// Integral numbers written in float form (1.0, 1e3) are still accepted, but
// only within the range where float64 is exact.
func parseInt(number json.Number) (int64, error) {
	value, err := strconv.ParseInt(number.String(), 10, 64)
	if err == nil {
		return value, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, errIntegerOverflow
	}
	f, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || math.Trunc(f) != f {
		return 0, errNotInteger
	}
	if math.Abs(f) > maxExactFloat {
		return 0, errIntegerOverflow
	}
	return int64(f), nil
}

// parseUint is the unsigned counterpart of parseInt.
func parseUint(number json.Number) (uint64, error) {
	value, err := strconv.ParseUint(number.String(), 10, 64)
	if err == nil {
		return value, nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, errIntegerOverflow
	}
	f, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || f < 0 || math.Trunc(f) != f {
		return 0, errNotUnsignedInteger
	}
	if f > maxExactFloat {
		return 0, errIntegerOverflow
	}
	return uint64(f), nil
}

// numbersToFloat converts json.Number values back to float64 so values stored
// into interface fields keep the same dynamic types encoding/json produces.
func numbersToFloat(data any) any {
	switch v := data.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return v
		}
		return f
	case []any:
		for i := range v {
			v[i] = numbersToFloat(v[i])
		}
		return v
	case map[string]any:
		for key, item := range v {
			v[key] = numbersToFloat(item)
		}
		return v
	default:
		return data
	}
}
