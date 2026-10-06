package jsoncompressor

import (
	"testing"
	"time"
)

// Known bugs pending a fix: remove the t.Skip calls once the fix lands so
// these become regression tests.

// Unmarshal decodes into `any` without UseNumber, so every number passes
// through float64 and integers above 2^53 silently change value.
func TestRoundTripLargeIntegers(t *testing.T) {
	t.Skip("known bug: decompress routes numbers through float64; integers above 2^53 lose precision")

	type payload struct {
		I int64  `json:"i"`
		U uint64 `json:"u"`
	}
	cases := []payload{
		{I: 9007199254740993, U: 9007199254740993},        // 2^53 + 1
		{I: 9223372036854774807, U: 18446744073709550615}, // near the 64-bit limits
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
