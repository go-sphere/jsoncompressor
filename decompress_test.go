package jsoncompressor

import (
	"testing"
)

func TestDecompress(t *testing.T) {
	{
		type MyStruct struct {
			K1 int    `json:"k1"`
			K2 string `json:"k2"`
			K3 []int  `json:"k3"`
			K5 *struct {
				K6 int    `json:"k6"`
				K8 string `json:"k8"`
			} `json:"k5"`
		}

		raw := []byte(`[1,"2",[3,4],[7,"8"]]`)
		var data MyStruct
		err := Unmarshal(raw, &data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		t.Logf("Unmarshaled: %+v", data)
		if data.K1 != 1 || data.K2 != "2" || len(data.K3) != 2 || data.K3[0] != 3 || data.K3[1] != 4 || data.K5.K6 != 7 || data.K5.K8 != "8" {
			t.Fatalf("Unmarshaled data is invalid")
		}

		dataV2 := &MyStruct{}
		err = Unmarshal(raw, &dataV2)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		t.Logf("Unmarshaled: %+v", dataV2)
	}
	{
		raw := []byte(`1`)
		var data int
		err := Unmarshal(raw, &data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		t.Logf("Unmarshaled: %+v", data)
	}
	{
		raw := []byte(`["test"]`)
		var data []string
		err := Unmarshal(raw, &data)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		t.Logf("Unmarshaled: %v", data)
	}
}

func TestDecompressTagWithoutNameAndLengthMismatch(t *testing.T) {
	type S struct {
		A int `json:",omitempty"`
		B int `json:"b"`
	}
	{
		raw := []byte(`[10,20]`)
		var s S
		if err := Unmarshal(raw, &s); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if s.A != 10 || s.B != 20 {
			t.Fatalf("Unexpected values: %+v", s)
		}
	}
	{
		raw := []byte(`[1,2,3]`)
		var s S
		if err := Unmarshal(raw, &s); err == nil {
			t.Fatalf("expected error on length mismatch (too many)")
		}
	}
	{
		raw := []byte(`[1]`)
		var s S
		if err := Unmarshal(raw, &s); err == nil {
			t.Fatalf("expected error on length mismatch (too few)")
		}
	}
}

func TestDecompressPointerInitialization(t *testing.T) {
	type Child struct {
		A int `json:"a"`
	}
	type S struct {
		Child *Child `json:"child"`
	}
	raw := []byte(`[[1]]`)
	var s S
	if err := Unmarshal(raw, &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s.Child == nil || s.Child.A != 1 {
		t.Fatalf("Unexpected values: %+v", s)
	}
}

func TestDecompressTypeMismatch(t *testing.T) {
	type S struct {
		A int `json:"a"`
	}
	raw := []byte(`["oops"]`)
	var s S
	if err := Unmarshal(raw, &s); err == nil {
		t.Fatalf("expected error on type mismatch")
	}
}

func TestDecompressUnsupportedKind(t *testing.T) {
	type S struct {
		C chan int `json:"c"`
	}
	raw := []byte(`[1]`)
	var s S
	if err := Unmarshal(raw, &s); err == nil {
		t.Fatalf("expected error for unsupported kind")
	}
}

func TestDecompressTargetValidation(t *testing.T) {
	if err := Unmarshal([]byte(`1`), 1); err == nil {
		t.Fatalf("expected error for non-pointer target")
	}
	var ptr *int
	if err := Unmarshal([]byte(`1`), ptr); err == nil {
		t.Fatalf("expected error for nil pointer target")
	}
}

func TestDecompressNullPointerField(t *testing.T) {
	type S struct {
		A *int `json:"a"`
	}
	raw := []byte(`[null]`)
	var s S
	if err := Unmarshal(raw, &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s.A != nil {
		t.Fatalf("expected nil pointer, got: %+v", s.A)
	}
}

func TestDecompressInterfaceAssignment(t *testing.T) {
	type S struct {
		A any `json:"a"`
	}
	raw := []byte(`["hello"]`)
	var s S
	if err := Unmarshal(raw, &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if s.A != "hello" {
		t.Fatalf("unexpected interface value: %+v", s.A)
	}
}

func TestDecompressSliceElementMismatch(t *testing.T) {
	type S struct {
		A []int `json:"a"`
	}
	raw := []byte(`[["oops"]]`)
	var s S
	if err := Unmarshal(raw, &s); err == nil {
		t.Fatalf("expected error for slice element mismatch")
	}
}

func TestDecompressUnsignedNegative(t *testing.T) {
	type S struct {
		A uint `json:"a"`
	}
	raw := []byte(`[-1]`)
	var s S
	if err := Unmarshal(raw, &s); err == nil {
		t.Fatalf("expected error for negative uint value")
	}
}

func TestDecompressNoTaggedFields(t *testing.T) {
	type S struct {
		A int
		B string
	}
	{
		raw := []byte(`[]`)
		var s S
		if err := Unmarshal(raw, &s); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
	}
	{
		raw := []byte(`[1]`)
		var s S
		if err := Unmarshal(raw, &s); err == nil {
			t.Fatalf("expected error for data without tagged fields")
		}
	}
}

func TestDecompressMap(t *testing.T) {
	{
		raw := []byte(`{"a":1,"b":2}`)
		var out map[string]int
		if err := Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if len(out) != 2 || out["a"] != 1 || out["b"] != 2 {
			t.Fatalf("unexpected map values: %+v", out)
		}
	}
	{
		raw := []byte(`{"nums":[1,2]}`)
		var out map[string][]int
		if err := Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if len(out["nums"]) != 2 || out["nums"][0] != 1 || out["nums"][1] != 2 {
			t.Fatalf("unexpected map values: %+v", out)
		}
	}
}
