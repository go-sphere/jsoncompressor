package jsoncompressor

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// Unmarshal used to decode into `any` without UseNumber, so every number
// passed through float64 and integers above 2^53 silently changed value.
func TestRoundTripLargeIntegers(t *testing.T) {
	type payload struct {
		I int64  `json:"i"`
		U uint64 `json:"u"`
	}
	cases := []payload{
		{I: 9007199254740993, U: 9007199254740993},        // 2^53 + 1
		{I: 9223372036854774807, U: 18446744073709550615}, // near the 64-bit limits
		{I: math.MaxInt64, U: math.MaxUint64},
		{I: math.MinInt64},
	}
	for _, want := range cases {
		raw, err := Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%+v) failed: %v", want, err)
		}
		var got payload
		if err := Unmarshal(raw, &got); err != nil {
			t.Fatalf("Unmarshal(%s) failed: %v", raw, err)
		}
		if got != want {
			t.Fatalf("round trip changed value: got %+v, want %+v (raw %s)", got, want, raw)
		}
	}
}

// compressValue used to walk struct fields by reflection without consulting
// json.Marshaler, so time.Time (all fields unexported) was compressed to []
// and decoded to the zero time without an error.
func TestRoundTripTime(t *testing.T) {
	type payload struct {
		At time.Time `json:"at"`
		N  int       `json:"n"`
	}
	want := payload{At: time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC), N: 7}
	raw, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var got payload
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal(%s) failed: %v", raw, err)
	}
	if !got.At.Equal(want.At) || got.N != want.N {
		t.Fatalf("round trip changed value: got %+v, want %+v (raw %s)", got, want, raw)
	}
}

func TestRoundTripTimeVariants(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)
	type payload struct {
		Ptr   *time.Time           `json:"ptr"`
		Nil   *time.Time           `json:"nil"`
		Slice []time.Time          `json:"slice"`
		Map   map[string]time.Time `json:"map"`
	}
	want := payload{Ptr: &at, Slice: []time.Time{at}, Map: map[string]time.Time{"k": at}}
	raw, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var got payload
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal(%s) failed: %v", raw, err)
	}
	if got.Ptr == nil || !got.Ptr.Equal(at) || got.Nil != nil ||
		len(got.Slice) != 1 || !got.Slice[0].Equal(at) || !got.Map["k"].Equal(at) {
		t.Fatalf("round trip changed value: got %+v (raw %s)", got, raw)
	}
}

type upperString string

func (s upperString) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToUpper(string(s)))
}

func (s *upperString) UnmarshalJSON(data []byte) error {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = upperString(strings.ToLower(v))
	return nil
}

func TestCustomMarshalerIsUsed(t *testing.T) {
	type payload struct {
		S upperString `json:"s"`
	}
	raw, err := Marshal(payload{S: "abc"})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `["ABC"]` {
		t.Fatalf("expected MarshalJSON output, got %s", raw)
	}
	var got payload
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.S != "abc" {
		t.Fatalf("expected UnmarshalJSON to run, got %q", got.S)
	}
}

func TestDecompressIntegerForms(t *testing.T) {
	type payload struct {
		I int64 `json:"i"`
	}
	ok := map[string]int64{
		`[1.0]`:               1,
		`[1e3]`:               1000,
		`[-9007199254740993]`: -9007199254740993,
	}
	for raw, want := range ok {
		var got payload
		if err := Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("Unmarshal(%s) failed: %v", raw, err)
		}
		if got.I != want {
			t.Fatalf("Unmarshal(%s) = %d, want %d", raw, got.I, want)
		}
	}
	bad := []string{
		`[1.5]`,                 // not an integer
		`[1e17]`,                // float form beyond the exact float64 range
		`[9223372036854775808]`, // MaxInt64 + 1
	}
	for _, raw := range bad {
		var got payload
		if err := Unmarshal([]byte(raw), &got); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded with %d, want error", raw, got.I)
		}
	}
}

func TestDecompressNarrowIntegerOverflow(t *testing.T) {
	type payload struct {
		I int8  `json:"i"`
		U uint8 `json:"u"`
	}
	for _, raw := range []string{`[128,0]`, `[0,256]`, `[0,18446744073709551616]`} {
		var got payload
		if err := Unmarshal([]byte(raw), &got); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded with %+v, want overflow error", raw, got)
		}
	}
}

func TestDecompressInterfaceKeepsFloat64(t *testing.T) {
	type payload struct {
		A any `json:"a"`
	}
	var got payload
	if err := Unmarshal([]byte(`[[1,{"x":2}]]`), &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	items, ok := got.A.([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("unexpected interface value: %#v", got.A)
	}
	if _, ok := items[0].(float64); !ok {
		t.Fatalf("expected float64, got %T", items[0])
	}
	if _, ok := items[1].(map[string]any)["x"].(float64); !ok {
		t.Fatalf("expected nested float64, got %#v", items[1])
	}
}

func TestDecompressRejectsTrailingData(t *testing.T) {
	var got []int
	if err := Unmarshal([]byte(`[1] [2]`), &got); err == nil {
		t.Fatalf("expected error for trailing data")
	}
	if err := Unmarshal([]byte("[1]\n"), &got); err != nil {
		t.Fatalf("trailing whitespace should be accepted: %v", err)
	}
}

// ptrCodec implements both methods on the pointer only.
type ptrCodec struct{ V int }

func (p *ptrCodec) MarshalJSON() ([]byte, error)    { return json.Marshal(p.V) }
func (p *ptrCodec) UnmarshalJSON(data []byte) error { return json.Unmarshal(data, &p.V) }

// decodeOnly implements only json.Unmarshaler, so it is expanded field by field.
type decodeOnly struct {
	V int `json:"v"`
}

func (d *decodeOnly) UnmarshalJSON([]byte) error { return errors.New("must not be called") }

func TestRoundTripMarshalerSymmetry(t *testing.T) {
	// Map values and values passed by value are not addressable; Marshal must
	// still use the pointer-receiver MarshalJSON that Unmarshal will pair with.
	mapWant := map[string]ptrCodec{"a": {V: 3}}
	raw, err := Marshal(mapWant)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `{"a":3}` {
		t.Fatalf("expected MarshalJSON output, got %s", raw)
	}
	var mapGot map[string]ptrCodec
	if err := Unmarshal(raw, &mapGot); err != nil || mapGot["a"].V != 3 {
		t.Fatalf("Unmarshal(%s) = %+v, %v", raw, mapGot, err)
	}

	type payload struct {
		D decodeOnly `json:"d"`
	}
	raw, err = Marshal(payload{D: decodeOnly{V: 5}})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var got payload
	if err := Unmarshal(raw, &got); err != nil || got.D.V != 5 {
		t.Fatalf("Unmarshal(%s) = %+v, %v", raw, got, err)
	}
}

func TestMarshalNil(t *testing.T) {
	raw, err := Marshal(nil)
	if err != nil || string(raw) != "null" {
		t.Fatalf("Marshal(nil) = %s, %v", raw, err)
	}
}
