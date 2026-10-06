# Changelog

## Unreleased

A silent data-corruption fix. Payloads written by earlier versions still
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

### Behaviour changes

- Trailing data after the top-level JSON value is still rejected, as before.
