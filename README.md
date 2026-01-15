# JsonCompressor

JsonCompressor is a lightweight Go library that compresses JSON structures into arrays by preserving only the values while maintaining the field order. It's particularly useful when you need to reduce JSON payload size while retaining the ability to reconstruct the original structure.

## Features

- Compress any struct with JSON tags into an array format
- Decompress arrays back into their original struct format
- Recursive handling of nested structures and arrays
- Preserves field order based on struct definition
- Type-safe conversion

## Installation

```bash
go get github.com/go-sphere/jsoncompressor
```

## Usage

```go
type Person struct {
    Name    string   `json:"name"`
    Age     int      `json:"age"`
    Hobbies []string `json:"hobbies"`
    Address struct {
        City    string `json:"city"`
        Country string `json:"country"`
    } `json:"address"`
}

// Create a sample struct
person := Person{
    Name:    "John Doe",
    Age:     30,
    Hobbies: []string{"reading", "coding"},
    Address: struct {
        City    string `json:"city"`
        Country string `json:"country"`
    }{
        City:    "New York",
        Country: "USA",
    },
}

// Compress
compressed, err := jsoncompressor.Marshal(person)
if err != nil {
    log.Fatal(err)
}
// Result: ["John Doe", 30, ["reading", "coding"], ["New York", "USA"]]

// Decompress
var decompressed Person
err = jsoncompressor.Unmarshal(compressed, &decompressed)
if err != nil {
    log.Fatal(err)
}
```

## Notes

- Only processes fields with JSON tags
- Field order must be consistent between compression and decompression
- Requires proper type matching for successful decompression

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
