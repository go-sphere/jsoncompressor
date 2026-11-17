package jsoncompressor

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unsafe"
)

const maxDepth = 1000

type visitKey struct {
	ptr    unsafe.Pointer
	typ    reflect.Type
	length int
}

// Marshal returns the compressed JSON encoding of v: each struct is written
// as an array of its tagged, exported field values in declaration order
// rather than as an object. Nested structs, pointers, slices, arrays, and
// string-keyed maps are compressed recursively; see the package
// documentation for the full rules.
//
// A nil v, or a nil pointer, encodes as null. A nil slice encodes as [].
// Marshal returns an error, and no data, when v contains a channel, function,
// unsafe pointer, uintptr, complex number, cyclic reference, or a map with
// non-string keys, when nesting exceeds the maximum depth, or when a custom
// MarshalJSON method or encoding/json fails. The errors are not sentinels.
//
// Decode the result with [Unmarshal] into the same Go type.
func Marshal(v any) ([]byte, error) {
	visited := make(map[visitKey]struct{})
	compressed, err := compressValue(reflect.ValueOf(v), 0, visited)
	if err != nil {
		return nil, err
	}
	return json.Marshal(compressed)
}

func compressStruct(val reflect.Value, depth int, visited map[visitKey]struct{}) ([]any, error) {
	meta := getStructMeta(val.Type())
	if meta == nil {
		// no fields or not a struct (defensive)
		return []any{}, nil
	}
	result := make([]any, 0, len(meta.fields))
	for _, idx := range meta.fields {
		v, err := compressValue(val.Field(idx), depth+1, visited)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func compressValue(v reflect.Value, depth int, visited map[visitKey]struct{}) (any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("exceeded max recursion depth (%d)", maxDepth)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		key := visitKey{ptr: v.UnsafePointer(), typ: v.Type()}
		if _, seen := visited[key]; seen {
			return nil, fmt.Errorf("cyclic pointer of type %s", v.Type())
		}
		visited[key] = struct{}{}
		defer delete(visited, key)
		return compressValue(v.Elem(), depth+1, visited)
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
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Uintptr, reflect.Complex64, reflect.Complex128:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		if v.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("unsupported map key type: %s", v.Type().Key())
		}
		if v.Len() > 0 {
			key := visitKey{ptr: v.UnsafePointer(), typ: v.Type()}
			if _, seen := visited[key]; seen {
				return nil, fmt.Errorf("cyclic map of type %s", v.Type())
			}
			visited[key] = struct{}{}
			defer delete(visited, key)
		}
		result := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			value, err := compressValue(iter.Value(), depth+1, visited)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		return result, nil
	case reflect.Struct:
		return compressStruct(v, depth, visited)
	case reflect.Slice, reflect.Array:
		length := v.Len()
		if kind == reflect.Slice && length > 0 {
			key := visitKey{ptr: v.UnsafePointer(), typ: v.Type(), length: length}
			if _, seen := visited[key]; seen {
				return nil, fmt.Errorf("cyclic slice of type %s", v.Type())
			}
			visited[key] = struct{}{}
			defer delete(visited, key)
		}
		result := make([]any, length)
		for i := range length {
			val, err := compressValue(v.Index(i), depth+1, visited)
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
