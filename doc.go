// Package jsoncompressor encodes Go structs as JSON arrays of field values
// instead of objects, dropping the field names to shrink payloads, and
// decodes such arrays back into the same struct types.
//
// Use [Marshal] to compress and [Unmarshal] to decompress. Both sides must
// use the same Go type, because a struct is identified only by the order of
// its fields.
//
// # Usage
//
//	import "github.com/go-sphere/jsoncompressor"
//
//	type Person struct {
//		Name string `json:"name"`
//		Age  int    `json:"age"`
//	}
//
//	data, err := jsoncompressor.Marshal(Person{Name: "Ada", Age: 36})
//	if err != nil {
//		return err
//	}
//	// data is ["Ada",36]
//
//	var p Person
//	if err := jsoncompressor.Unmarshal(data, &p); err != nil {
//		return err
//	}
//
// # Encoding rules
//
//   - A struct becomes an array of its exported fields that have a non-empty
//     json tag other than "-", in declaration order. Untagged fields,
//     including embedded structs, are skipped. Tag options such as omitempty
//     are ignored, and a tag with an empty name, such as json:",omitempty",
//     still includes the field.
//   - Slices and arrays become arrays; maps with string keys become objects;
//     a nil pointer or nil map becomes null.
//   - A type whose pointer implements both encoding/json.Marshaler and
//     encoding/json.Unmarshaler, such as time.Time, keeps its own JSON form.
//   - Other values, including values held in interface fields, are encoded
//     by encoding/json unchanged; structs inside an interface are therefore
//     written as ordinary JSON objects.
//   - Channels, functions, unsafe pointers, uintptr values, and maps with
//     non-string keys are rejected with an error.
//
// Marshal and Unmarshal cache per-type metadata internally and are safe for
// concurrent use.
package jsoncompressor
