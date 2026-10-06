package jsoncompressor

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Marshal returns the compressed JSON encoding of v: each struct is written
// as an array of its tagged, exported field values in declaration order
// rather than as an object. Nested structs, pointers, slices, arrays, and
// string-keyed maps are compressed recursively; see the package
// documentation for the full rules.
//
// A nil v, or a nil pointer, encodes as null. A nil slice encodes as [].
// Marshal returns an error, and no data, when v contains a channel, function,
// unsafe pointer, uintptr, or a map with non-string keys, or when a custom
// MarshalJSON method or encoding/json fails. The errors are not sentinels.
//
// Decode the result with [Unmarshal] into the same Go type.
func Marshal(v any) ([]byte, error) {
	compressed, err := compressValue(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	return json.Marshal(compressed)
}

func compressStruct(val reflect.Value) ([]any, error) {
	meta := getStructMeta(val.Type())
	if meta == nil {
		// no fields or not a struct (defensive)
		return []any{}, nil
	}
	result := make([]any, 0, len(meta.fields))
	for _, f := range meta.fields {
		v, err := compressValue(val.Field(f.index))
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func compressValue(v reflect.Value) (any, error) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil, nil
	}
	if ownsJSON(v.Type()) {
		// Types such as time.Time own their JSON form; their fields are
		// usually unexported, so walking them would silently produce [].
		if !v.CanAddr() {
			// Copy so pointer-receiver methods are honoured for map values
			// and values passed by value, matching what Unmarshal decodes.
			p := reflect.New(v.Type())
			p.Elem().Set(v)
			v = p.Elem()
		}
		raw, err := v.Addr().Interface().(json.Marshaler).MarshalJSON()
		if err != nil {
			return nil, err
		}
		return json.RawMessage(raw), nil
	}
	kind := v.Kind()
	switch kind {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Uintptr:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		if v.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type: %s", v.Type().Key())
		}
		result := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			value, err := compressValue(iter.Value())
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case reflect.Struct:
		return compressStruct(v)
	case reflect.Slice, reflect.Array:
		length := v.Len()
		result := make([]any, length)
		for i := range length {
			val, err := compressValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil
	default:
		return v.Interface(), nil
	}
}
