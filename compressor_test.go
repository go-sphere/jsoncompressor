package jsoncompressor

import (
	"testing"
)

func TestCompress(t *testing.T) {
	{
		type MyStruct struct {
			K1 int    `json:"k1"`
			K2 string `json:"k2"`
			K3 []int  `json:"k3"`
			K5 struct {
				K6 int    `json:"k6"`
				K8 string `json:"k8"`
			} `json:"k5"`
		}

		data := MyStruct{
			K1: 1,
			K2: "2",
			K3: []int{3, 4},
			K5: struct {
				K6 int    `json:"k6"`
				K8 string `json:"k8"`
			}{
				K6: 7,
				K8: "8",
			},
		}
		raw, err := Marshal(data)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		target := `[1,"2",[3,4],[7,"8"]]`
		if string(raw) != target {
			t.Fatalf("Marshaled data is invalid, got: %s", raw)
		}
		t.Logf("Marshaled: %s", raw)
	}
	{
		raw, err := Marshal(1)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		target := `1`
		if string(raw) != target {
			t.Fatalf("Marshaled data is invalid, got: %s", raw)
		}
		t.Logf("Marshaled: %s", raw)
	}
	{
		raw, err := Marshal([]string{"test", "test2"})
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		target := `["test","test2"]`
		if string(raw) != target {
			t.Fatalf("Marshaled data is invalid, got: %s", raw)
		}
		t.Logf("Marshaled: %s", raw)
	}
}

func TestCompressSkipsUnexported(t *testing.T) {
	type MyStruct struct {
		A int `json:"a"`
		b string
		C int `json:"c"`
	}
	data := MyStruct{A: 1, b: "hidden", C: 3}
	raw, err := Marshal(data)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// unexported field 'b' should not be present
	target := `[1,3]`
	if string(raw) != target {
		t.Fatalf("Marshaled data is invalid, got: %s", raw)
	}
}

func TestCompressTagFilteringAndPointers(t *testing.T) {
	type Inner struct {
		C int `json:"c"`
	}
	type S struct {
		A *int   `json:",omitempty"`
		B string `json:"-"`
		C Inner  `json:"c"`
	}
	value := 10
	data := S{A: &value, B: "skip", C: Inner{C: 30}}
	raw, err := Marshal(data)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	target := `[10,[30]]`
	if string(raw) != target {
		t.Fatalf("Marshaled data is invalid, got: %s", raw)
	}

	dataNil := S{A: nil, B: "skip", C: Inner{C: 30}}
	raw, err = Marshal(dataNil)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	target = `[null,[30]]`
	if string(raw) != target {
		t.Fatalf("Marshaled data is invalid, got: %s", raw)
	}
}

func TestCompressStructWithoutTags(t *testing.T) {
	type S struct {
		A int
		B string
	}
	raw, err := Marshal(S{A: 1, B: "ignored"})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `[]` {
		t.Fatalf("expected empty array, got: %s", raw)
	}
}

func TestCompressMap(t *testing.T) {
	data := map[string]int{"b": 2, "a": 1}
	raw, err := Marshal(data)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(raw) != `{"a":1,"b":2}` {
		t.Fatalf("unexpected map output: %s", raw)
	}
}
