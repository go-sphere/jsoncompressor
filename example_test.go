package jsoncompressor_test

import (
	"fmt"
	"time"

	"github.com/go-sphere/jsoncompressor"
)

type Address struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type Person struct {
	Name    string   `json:"name"`
	Age     int      `json:"age"`
	Hobbies []string `json:"hobbies"`
	Address *Address `json:"address"`
	Note    string   // untagged: never encoded
}

func Example() {
	in := Person{
		Name:    "Ada",
		Age:     36,
		Hobbies: []string{"math", "poetry"},
		Address: &Address{City: "London", Country: "UK"},
		Note:    "dropped",
	}
	data, err := jsoncompressor.Marshal(in)
	if err != nil {
		fmt.Println("marshal:", err)
		return
	}
	fmt.Println(string(data))

	var out Person
	if err := jsoncompressor.Unmarshal(data, &out); err != nil {
		fmt.Println("unmarshal:", err)
		return
	}
	fmt.Println(out.Name, out.Age, out.Hobbies, out.Address.City, out.Note == "")
	// Output:
	// ["Ada",36,["math","poetry"],["London","UK"]]
	// Ada 36 [math poetry] London true
}

func ExampleMarshal_jsonMarshaler() {
	// time.Time implements json.Marshaler and json.Unmarshaler, so it keeps
	// its own JSON form instead of being expanded into an array.
	type Event struct {
		Name string    `json:"name"`
		At   time.Time `json:"at"`
	}
	data, err := jsoncompressor.Marshal(Event{Name: "launch", At: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)})
	if err != nil {
		fmt.Println("marshal:", err)
		return
	}
	fmt.Println(string(data))
	// Output: ["launch","2024-01-02T03:04:05Z"]
}

func ExampleUnmarshal_fieldCountMismatch() {
	// The array must have exactly one value per tagged field.
	var a Address
	err := jsoncompressor.Unmarshal([]byte(`["London"]`), &a)
	fmt.Println(err)
	// Output: field count mismatch: have 1 values, want 2
}
