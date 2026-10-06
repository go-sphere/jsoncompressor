package jsoncompressor

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

type fieldMeta struct {
	index    int
	jsonName string
}

type structMeta struct {
	fields []fieldMeta // ordered, only exported fields with json tag (not "-")
}

var (
	typeCache sync.Map // map[reflect.Type]*structMeta

	marshalerCache   sync.Map // map[reflect.Type]bool, keyed by the pointer type
	unmarshalerCache sync.Map // map[reflect.Type]bool, keyed by the pointer type

	marshalerType   = reflect.TypeFor[json.Marshaler]()
	unmarshalerType = reflect.TypeFor[json.Unmarshaler]()
)

// implementsMarshaler reports whether an addressable value of type t
// implements json.Marshaler, either directly or through its pointer.
func implementsMarshaler(t reflect.Type) bool {
	return cachedImplements(&marshalerCache, reflect.PointerTo(t), marshalerType)
}

// implementsUnmarshaler reports whether *t implements json.Unmarshaler.
func implementsUnmarshaler(t reflect.Type) bool {
	return cachedImplements(&unmarshalerCache, reflect.PointerTo(t), unmarshalerType)
}

func cachedImplements(cache *sync.Map, t, iface reflect.Type) bool {
	if v, ok := cache.Load(t); ok {
		return v.(bool)
	}
	ok := t.Implements(iface)
	cache.Store(t, ok)
	return ok
}

func getStructMeta(t reflect.Type) *structMeta {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := typeCache.Load(t); ok {
		return v.(*structMeta)
	}
	// Build metadata
	num := t.NumField()
	fields := make([]fieldMeta, 0, num)
	for i := range num {
		ft := t.Field(i)
		name, ok := getJsonKey(&ft)
		if !ok {
			continue
		}
		fields = append(fields, fieldMeta{index: i, jsonName: name})
	}
	m := &structMeta{fields: fields}
	if existing, loaded := typeCache.LoadOrStore(t, m); loaded {
		return existing.(*structMeta)
	}
	return m
}

func getJsonKey(field *reflect.StructField) (string, bool) {
	if !field.IsExported() { // unexported
		return "", false
	}
	tag := field.Tag.Get("json")
	if tag == "-" || tag == "" {
		return "", false
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		name = field.Name
	}
	return name, true
}
