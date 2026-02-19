package jsoncompressor

import (
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
)

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
