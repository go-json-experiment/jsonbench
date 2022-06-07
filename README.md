# JSON Benchmarks

Each of the charts below show the performance across
several different JSON implementations:

* `JSONv1` is `encoding/json` at `v1.18.2`
* `JSONv2` is `github.com/go-json-experiment/json` at `v0.0.0-20220524042235-dd8be80fc4a7`
* `JSONIterator` is `github.com/json-iterator/go` at `v1.1.12`
* `SegmentJSON` is `github.com/segmentio/encoding/json` at `v0.3.5`
* `GoJSON` is `github.com/goccy/go-json` at `v0.9.7`
* `SonicJSON` is `github.com/bytedance/sonic` at `v1.3.0`

Benchmarks were run across various datasets:

* `CanadaGeometry` is a GeoJSON (RFC 7946) representation of Canada.
  It contains many JSON arrays of arrays of two-element arrays of numbers.
* `CITMCatalog` contains many JSON objects using numeric names.
* `SyntheaFHIR` is sample JSON data from the healthcare industry.
  It contains many nested JSON objects with mostly string values,
  where the set of unique string values is relatively small.
* `TwitterStatus` is the JSON response from the Twitter API.
  It contains a mix of all different JSON kinds, where string values
  are a mix of both single-byte ASCII and multi-byte Unicode.
* `GolangSource` is a simple tree representing the Go source code.
  It contains many nested JSON objects, each with the same schema.
* `StringUnicode` contains many strings with multi-byte Unicode runes.

All of the implementations other than `JSONv1` and `JSONv2` make
extensive use of `unsafe`. As such, we expect those to generally be faster,
but at the cost of memory and type safety. `SonicJSON` goes a step even further
and uses just-in-time compilation to generate machine code specialized
for the Go type being marshaled or unmarshaled.
Also, `SonicJSON` does not validate JSON strings for valid UTF-8,
and so gains a notable performance boost on datasets with multi-byte Unicode.
Benchmarks are performed based on the default marshal and unmarshal behavior
of each package. Note that `JSONv2` aims to be safe and correct by default,
which may not be the most performant strategy.

`JSONv2` has several semantic changes relative to `JSONv1` that
impacts performance:

1.  When marshaling, `JSONv2` no longer sorts the keys of a Go map.
    This will improve performance.
2.  When marshaling or unmarshaling, `JSONv2` always checks
    to make sure JSON object names are unique.
    This will hurt performance, but is more correct.
3.  When marshaling or unmarshaling, `JSONv2` always
    shallow copies the underlying value for a Go interface and
    shallow copies the key and value for entries in a Go map.
    This is done to keep the value as addressable so that `JSONv2` can
    call methods and functions that operate on a pointer receiver.
    This will hurt performance, but is more correct.

All of the charts are unit-less since the values are normalized
relative to `JSONv1`, which is why `JSONv1` always has a value of 1.
A lower value is better (i.e., runs faster).

## Marshal Performance

### Concrete types

![image](/images/benchmark-marshal-concrete.png)

* This compares marshal performance when serializing
  [from concrete types](/testdata_test.go).
* The `JSONv1` implementation is close to optimal (without the use of `unsafe`).
* Relative to `JSONv1`, `JSONv2` is generally as fast or slightly faster.
* Relative to `JSONIterator`, `JSONv2` is up to 1.3x faster.
* Relative to `SegmentJSON`, `JSONv2` is up to 1.8x slower.
* Relative to `GoJSON`, `JSONv2` is up to 2.0x slower.
* Relative to `SonicJSON`, `JSONv2` is about 1.8x to 3.2x slower
  (ignoring `StringUnicode` since `SonicJSON` does not validate UTF-8).
* For `JSONv1` and `JSONv2`, marshaling from concrete types is
  mostly limited by the performance of Go reflection.

### Interface types

![image](/images/benchmark-marshal-interface.png)

* This compares marshal performance when serializing from
  `any`, `map[string]any`, and `[]any` types.
* Relative to `JSONv1`, `JSONv2` is about 1.5x to 4.2x faster.
* Relative to `JSONIterator`, `JSONv2` is about 1.1x to 2.4x faster.
* Relative to `SegmentJSON`, `JSONv2` is about 1.2x to 1.8x faster.
* Relative to `GoJSON`, `JSONv2` is about 1.1x to 2.5x faster.
* Relative to `SonicJSON`, `JSONv2` is up to 1.5x slower
  (ignoring `StringUnicode` since `SonicJSON` does not validate UTF-8).
* `JSONv2` is faster than the alternatives.
  One advantange is because it does not sort the keys for a `map[string]any`,
  while alternatives (except `SonicJSON` and `JSONIterator`) do sort the keys.

## Raw Value types

![image](/images/benchmark-marshal-rawvalue.png)

* This compares performance when marshaling from a `json.RawValue`.
  This mostly exercises the underlying encoder and
  hides the cost of Go reflection.
* Relative to `JSONv1`, `JSONv2` is about 3.5x to 7.8x faster.
* `JSONIterator` is blazingly fast because
  [it does not validate whether the raw value is valid](https://go.dev/play/p/bun9IXQCKRe)
  and simply copies it to the output.
* Relative to `SegmentJSON`, `JSONv2` is about 1.5x to 2.7x faster.
* Relative to `GoJSON`, `JSONv2` is up to 2.2x faster.
* Relative to `SonicJSON`, `JSONv2` is up to 1.5x faster.
* Aside from `JSONIterator`, `JSONv2` is generally the fastest.

# Unmarshal Performance

## Concrete types

![image](/images/benchmark-unmarshal-concrete.png)

* This compares unmarshal performance when deserializing
  [into concrete types](/testdata_test.go).
* Relative to `JSONv1`, `JSONv2` is about 1.8x to 5.7x faster.
* Relative to `JSONIterator`, `JSONv2` is about 1.1x to 1.6x slower.
* Relative to `SegmentJSON`, `JSONv2` is up to 2.5x slower.
* Relative to `GoJSON`, `JSONv2` is about 1.4x to 2.1x slower.
* Relative to `SonicJSON`, `JSONv2` is up to 4.0x slower
  (ignoring `StringUnicode` since `SonicJSON` does not validate UTF-8).
* For `JSONv1` and `JSONv2`, unmarshaling into concrete types is
  mostly limited by the performance of Go reflection.

## Interface types

![image](/images/benchmark-unmarshal-interface.png)

* This compares unmarshal performance when deserializing into
  `any`, `map[string]any`, and `[]any` types.
* Relative to `JSONv1`, `JSONv2` is about 1.tx to 4.3x faster.
* Relative to `JSONIterator`, `JSONv2` is up to 1.5x faster.
* Relative to `SegmentJSON`, `JSONv2` is about 1.5 to 3.7x faster.
* Relative to `GoJSON`, `JSONv2` is up to 1.3x faster.
* Relative to `SonicJSON`, `JSONv2` is up to 1.5x slower
  (ignoring `StringUnicode` since `SonicJSON` does not validate UTF-8).
* Aside from `SonicJSON`, `JSONv2` is generally just as fast
  or faster than all the alternatives.

## Raw Value types

![image](/images/benchmark-unmarshal-rawvalue.png)

* This compares performance when unmarshaling into a `json.RawValue`.
  This mostly exercises the underlying decoder and
  hides away most of the cost of Go reflection.
* Relative to `JSONv1`, `JSONv2` is about 8.3x to 17.0x faster.
* Relative to `JSONIterator`, `JSONv2` is up to 2.0x faster.
* Relative to `SegmentJSON`, `JSONv2` is up to 1.6x faster or 1.7x slower.
* Relative to `GoJSON`, `JSONv2` is up to 1.9x faster or 2.1x slower.
* Relative to `SonicJSON`, `JSONv2` is up to 2.0x faster
  (ignoring `StringUnicode` since `SonicJSON` does not validate UTF-8).
* `JSONv1` takes a
  [lexical scanning approach](https://talks.golang.org/2011/lex.slide#1),
  which performs a virtual function call for every byte of input.
  In contrast, `JSONv2` makes heavy use of iterative and linear parsing logic
  (with extra complexity to resume parsing when encountering segmented buffers).
* `JSONv2` is comparable to the alternatives that use `unsafe`.
  Generally it is faster, but sometimes it is slower.
