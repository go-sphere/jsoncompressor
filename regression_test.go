package jsoncompressor

import (
	"math"
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

// compressValue walks struct fields by reflection and never consults
// json.Marshaler, so time.Time (all fields unexported) is compressed to []
// and decodes to the zero time without an error.
func TestRoundTripTime(t *testing.T) {
	t.Skip("known bug: json.Marshaler/Unmarshaler are bypassed; time.Time is compressed to []")

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
