package jsoncompressor

import (
	"bytes"
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
		`[1.0]`:                      1,
		`[1e3]`:                      1000,
		`[2.5e1]`:                    25,
		`[-0.0]`:                     0,
		`[-9007199254740993]`:        -9007199254740993,
		`[9007199254740993.0]`:       9007199254740993,
		`[0.9223372036854775807e19]`: math.MaxInt64,
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
		`[1.5]`,                   // not an integer
		`[4503599627370497.5]`,    // rounds to an integer as float64
		`[1.0000000000000000001]`, // likewise
		`[9223372036854775808]`,   // MaxInt64 + 1
		`[1e19]`,                  // float form beyond int64
		`[1e999999999999999999999]`,
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

func TestDecompressNonEmptyInterface(t *testing.T) {
	type payload struct {
		Err error `json:"err"`
	}
	for _, raw := range []string{`["boom"]`, `[123]`, `[{}]`} {
		var got payload
		if err := Unmarshal([]byte(raw), &got); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded with %+v, want error", raw, got)
		}
	}

	got := payload{Err: errors.New("stale")}
	if err := Unmarshal([]byte(`[null]`), &got); err != nil {
		t.Fatalf("Unmarshal([null]) failed: %v", err)
	}
	if got.Err != nil {
		t.Fatalf("expected nil Err on null, got %v", got.Err)
	}
}

func TestRoundTripRawMessageNull(t *testing.T) {
	type payload struct {
		Raw     json.RawMessage  `json:"raw"`
		Payload json.RawMessage  `json:"payload"`
		Ptr     *json.RawMessage `json:"ptr"`
	}
	want := payload{
		Raw:     json.RawMessage("null"),
		Payload: json.RawMessage(`{"k":1}`),
		Ptr:     nil,
	}
	raw, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `[null,{"k":1},null]` {
		t.Fatalf("unexpected compressed output: %s", raw)
	}
	var got payload
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal(%s) failed: %v", raw, err)
	}
	if got.Raw == nil || !bytes.Equal(got.Raw, []byte("null")) {
		t.Fatalf("expected non-nil json.RawMessage(\"null\"), got %#v", got.Raw)
	}
	if !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("expected %s, got %s", want.Payload, got.Payload)
	}
	if got.Ptr != nil {
		t.Fatalf("expected nil *json.RawMessage, got %#v", got.Ptr)
	}
}

type OptionalInt struct {
	Set   bool
	Valid bool
	Value int
}

func (o OptionalInt) MarshalJSON() ([]byte, error) {
	if !o.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

func (o *OptionalInt) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(data, []byte("null")) {
		o.Valid = false
		o.Value = 0
		return nil
	}
	o.Valid = true
	return json.Unmarshal(data, &o.Value)
}

func TestCustomNullableValueReceivesNull(t *testing.T) {
	type payload struct {
		NullVal OptionalInt  `json:"null_val"`
		SetVal  OptionalInt  `json:"set_val"`
		Ptr     *OptionalInt `json:"ptr"`
	}
	in := payload{
		NullVal: OptionalInt{Set: true, Valid: false},
		SetVal:  OptionalInt{Set: true, Valid: true, Value: 42},
		Ptr:     nil,
	}
	raw, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `[null,42,null]` {
		t.Fatalf("unexpected marshaled form: %s", raw)
	}

	got := payload{
		Ptr: &OptionalInt{Set: true, Valid: true, Value: 99},
	}
	if err := Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !got.NullVal.Set || got.NullVal.Valid || got.NullVal.Value != 0 {
		t.Fatalf("expected UnmarshalJSON(null) to mark Set=true, Valid=false; got %+v", got.NullVal)
	}
	if !got.SetVal.Set || !got.SetVal.Valid || got.SetVal.Value != 42 {
		t.Fatalf("unexpected SetVal: %+v", got.SetVal)
	}
	if got.Ptr != nil {
		t.Fatalf("expected nil pointer for Ptr on null, got %+v", got.Ptr)
	}
}

func TestDecompressExtremeExponents(t *testing.T) {
	type intPayload struct {
		I int64 `json:"i"`
	}
	type uintPayload struct {
		U uint64 `json:"u"`
	}

	var gotInt intPayload
	if err := Unmarshal([]byte(`[1e9223372036854775807]`), &gotInt); !errors.Is(err, errIntegerOverflow) {
		t.Fatalf("expected errIntegerOverflow for 1eMaxInt64, got %v", err)
	}
	if err := Unmarshal([]byte(`[0.01e-9223372036854775808]`), &gotInt); !errors.Is(err, errNotInteger) {
		t.Fatalf("expected errNotInteger for 0.01eMinInt64, got %v", err)
	}
	if err := Unmarshal([]byte(`[1e-99999999999999999999999]`), &gotInt); !errors.Is(err, errNotInteger) {
		t.Fatalf("expected errNotInteger for 1e-huge, got %v", err)
	}

	var gotUint uintPayload
	if err := Unmarshal([]byte(`[1e9223372036854775807]`), &gotUint); !errors.Is(err, errIntegerOverflow) {
		t.Fatalf("expected errIntegerOverflow for uint 1eMaxInt64, got %v", err)
	}
	if err := Unmarshal([]byte(`[0.01e-9223372036854775808]`), &gotUint); !errors.Is(err, errNotInteger) {
		t.Fatalf("expected errNotInteger for uint 0.01eMinInt64, got %v", err)
	}

	for _, raw := range []string{`[0e9223372036854775807]`, `[0.00e-9223372036854775808]`, `[-0e999999999999999999999]`} {
		gotInt.I = 7
		if err := Unmarshal([]byte(raw), &gotInt); err != nil {
			t.Fatalf("Unmarshal(%s) into int64 failed: %v", raw, err)
		}
		if gotInt.I != 0 {
			t.Fatalf("Unmarshal(%s) = %d, want 0", raw, gotInt.I)
		}
		gotUint.U = 7
		if err := Unmarshal([]byte(raw), &gotUint); err != nil {
			t.Fatalf("Unmarshal(%s) into uint64 failed: %v", raw, err)
		}
		if gotUint.U != 0 {
			t.Fatalf("Unmarshal(%s) = %d, want 0", raw, gotUint.U)
		}
	}
}

func TestDecompressInterfaceRejectsFloatOverflow(t *testing.T) {
	var top any
	if err := Unmarshal([]byte(`[1e1000]`), &top); err == nil {
		t.Fatalf("expected error unmarshaling [1e1000] into any, got %#v", top)
	}

	type payload struct {
		A any `json:"a"`
	}
	for _, raw := range []string{`[1e1000]`, `[[1e1000]]`, `[{"k":1e1000}]`} {
		var got payload
		if err := Unmarshal([]byte(raw), &got); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded with %#v, want error", raw, got)
		}
	}
}

func TestUnsupportedComplexKind(t *testing.T) {
	type c64 struct {
		V complex64 `json:"v"`
	}
	type c128 struct {
		V complex128 `json:"v"`
	}
	if _, err := Marshal(complex(1, 2)); err == nil {
		t.Fatalf("expected Marshal error for complex128")
	}
	if _, err := Marshal(c64{V: complex(1, 2)}); err == nil {
		t.Fatalf("expected Marshal error for complex64 struct field")
	}
	var out64 c64
	if err := Unmarshal([]byte(`[1]`), &out64); err == nil {
		t.Fatalf("expected Unmarshal error for complex64")
	}
	var out128 c128
	if err := Unmarshal([]byte(`[1]`), &out128); err == nil {
		t.Fatalf("expected Unmarshal error for complex128")
	}
}

func TestMarshalCyclesAndDepth(t *testing.T) {
	type node struct {
		Next *node `json:"next"`
	}
	// Direct pointer cycle
	a := &node{}
	a.Next = a
	if _, err := Marshal(a); err == nil {
		t.Fatalf("expected error on self-referential pointer")
	}

	// Mutual pointer cycle
	b := &node{}
	a.Next = b
	b.Next = a
	if _, err := Marshal(a); err == nil {
		t.Fatalf("expected error on mutual pointer cycle")
	}

	// Pure pointer-to-pointer cycle
	type selfPtr *selfPtr
	var sp selfPtr
	sp = &sp
	if _, err := Marshal(sp); err == nil {
		t.Fatalf("expected error on pointer-to-self cycle")
	}

	// Cyclic slice and map
	type recSlice []recSlice
	rs := make(recSlice, 1)
	rs[0] = rs
	if _, err := Marshal(rs); err == nil {
		t.Fatalf("expected error on cyclic slice")
	}
	type recMap map[string]recMap
	rm := recMap{}
	rm["self"] = rm
	if _, err := Marshal(rm); err == nil {
		t.Fatalf("expected error on cyclic map")
	}

	// Deep non-cyclic chain exceeding maxDepth
	head := &node{}
	cur := head
	for range maxDepth + 1 {
		next := &node{}
		cur.Next = next
		cur = next
	}
	if _, err := Marshal(head); err == nil {
		t.Fatalf("expected error when depth exceeds maxDepth")
	}

	// Shared non-cyclic pointer (DAG) must still succeed
	leaf := &node{}
	raw, err := Marshal([2]*node{leaf, leaf})
	if err != nil {
		t.Fatalf("shared non-cyclic pointer failed: %v", err)
	}
	if string(raw) != `[[null],[null]]` {
		t.Fatalf("unexpected shared pointer output: %s", raw)
	}

	// Pointer to offset-0 field inside outer struct must not trigger a false cycle
	type inner struct {
		X int `json:"x"`
	}
	type outer struct {
		First inner  `json:"first"`
		Ptr   *inner `json:"ptr"`
	}
	o := &outer{First: inner{X: 1}}
	o.Ptr = &o.First
	raw, err = Marshal(o)
	if err != nil {
		t.Fatalf("pointer to offset-0 field failed: %v", err)
	}
	if string(raw) != `[[1],[1]]` {
		t.Fatalf("unexpected offset-0 pointer output: %s", raw)
	}
}
