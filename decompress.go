package jsoncompressor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// Unmarshal decodes data produced by [Marshal] into target, which must be a
// non-nil pointer to the same Go type that was marshaled.
//
// Each JSON array is matched to a struct's tagged, exported fields by
// position, and its length must equal the number of such fields exactly.
// JSON null sets the destination to its zero value (a nil pointer for pointer
// fields). Nil pointers are allocated as needed. Slices are replaced; decoded
// map entries are added to an existing map without clearing it. Integer
// fields are decoded exactly from the number's text, so values above 2^53
// keep their precision; integral literals such as 1.0 or 1e3 are accepted,
// while fractional or out-of-range numbers are errors. Interface fields
// receive the same dynamic types as with encoding/json (float64 for numbers).
//
// Unmarshal returns an error when target is not a non-nil pointer, when data
// is not a single valid JSON value, or when the shape or types do not match.
// The errors are not sentinels. On error, target may have been partially
// updated.
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

var (
	errNotInteger         = errors.New("expected integer")
	errNotUnsignedInteger = errors.New("expected unsigned integer")
	errIntegerOverflow    = errors.New("integer overflow")
)

// parseInt converts a JSON number to int64 without going through float64.
func parseInt(number json.Number) (int64, error) {
	text, err := integerText(number.String())
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, errIntegerOverflow
	}
	return value, nil
}

// parseUint is the unsigned counterpart of parseInt.
func parseUint(number json.Number) (uint64, error) {
	text, err := integerText(number.String())
	if err != nil {
		return 0, err
	}
	if text == "-0" {
		text = "0"
	} else if strings.HasPrefix(text, "-") {
		return 0, errNotUnsignedInteger
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, errIntegerOverflow
	}
	return value, nil
}

// integerText rewrites a JSON number literal in float form (1.0, 1e3, 25e-1)
// as plain integer digits, working on the text so the result is exact. It
// returns errNotInteger when the literal has a fractional part.
func integerText(s string) (string, error) {
	if !strings.ContainsAny(s, ".eE") {
		return s, nil
	}
	sign := ""
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		sign, s = "-", rest
	}
	exp := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return "", errIntegerOverflow // exponent beyond int range
		}
		s, exp = s[:i], e
	}
	intPart, frac, _ := strings.Cut(s, ".")
	digits := strings.TrimRight(intPart+frac, "0")
	point := len(intPart) + exp // digits[:point] is the integer part
	trimmed := strings.TrimLeft(digits, "0")
	point -= len(digits) - len(trimmed)
	digits = trimmed
	switch {
	case digits == "":
		return "0", nil
	case point < len(digits):
		return "", errNotInteger
	case point > 20: // more digits than any 64-bit integer
		return "", errIntegerOverflow
	}
	return sign + digits + strings.Repeat("0", point-len(digits)), nil
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
