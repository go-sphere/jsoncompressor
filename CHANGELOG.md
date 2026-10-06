# Changelog

## Unreleased

Two silent data-corruption fixes. Payloads written by earlier versions still
decode: the compressed wire format for numbers is unchanged.

### Fixes

**Integers above 2^53 keep their value**

`Unmarshal` decoded every number through `float64`, so `int64`/`uint64`
values beyond 2^53 (snowflake IDs, for example) silently came back rounded:
`9007199254740993` decoded as `9007199254740992`, with no error. Numbers are
now decoded from their literal text. Integral values written in float form
(`1.0`, `1e3`) are still accepted, but only within the range where `float64`
is exact; beyond it, and beyond the target type's range, `Unmarshal` returns
an error instead of a wrong value.

Values stored into `interface{}` fields are unchanged: numbers there are
still `float64`, as with `encoding/json`.

**`json.Marshaler` / `json.Unmarshaler` are honoured**

Struct values were always expanded field by field, so types that own their
JSON form were bypassed. `time.Time`, whose fields are all unexported, was
compressed to `[]` and decoded to the zero time without an error. Types
implementing `json.Marshaler` are now encoded with `MarshalJSON`, and types
whose pointer implements `json.Unmarshaler` are decoded with `UnmarshalJSON`,
following `encoding/json`'s rules for pointer receivers.

### Behaviour changes

- The compressed output of types implementing `json.Marshaler` changes: a
  `time.Time` field is now its RFC 3339 string instead of `[]`. Payloads
  holding the old `[]` form for such a field now fail to decode rather than
  silently yielding a zero value.
- Trailing data after the top-level JSON value is still rejected, as before.
