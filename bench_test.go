// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonbench

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	jsonv1 "encoding/json"

	sonicjson "github.com/bytedance/sonic"
	sonicdec "github.com/bytedance/sonic/decoder"
	sonicenc "github.com/bytedance/sonic/encoder"
	jsonv2 "github.com/go-json-experiment/json"
	jsontext "github.com/go-json-experiment/json/jsontext"
	jsonv1in2 "github.com/go-json-experiment/json/v1"
	gojson "github.com/goccy/go-json"
	jsoniter "github.com/json-iterator/go"
	segjson "github.com/segmentio/encoding/json"
	sonnetjson "github.com/sugawarayuuta/sonnet"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tailscale/hujson"
	"tailscale.com/util/must"
)

func mustRead(path string) []byte {
	b := must.Get(os.ReadFile(path))
	zr := must.Get(gzip.NewReader(bytes.NewReader(b)))
	b = must.Get(io.ReadAll(zr))
	return b
}

func equalRawValue(x, y jsontext.Value) bool {
	normalize := func(v jsontext.Value) jsontext.Value {
		if v == nil {
			v = jsontext.Value("null")
		}
		v = v.Clone()
		v.Canonicalize()
		return v
	}
	return bytes.Equal(normalize(x), normalize(y))
}

var testdata = []struct {
	name string
	new  func() any
	data []byte
}{
	{"CanadaGeometry", func() any { return new(canadaRoot) }, mustRead("testdata/canada_geometry.json.gz")},
	{"CITMCatalog", func() any { return new(citmRoot) }, mustRead("testdata/citm_catalog.json.gz")},
	{"SyntheaFHIR", func() any { return new(syntheaRoot) }, mustRead("testdata/synthea_fhir.json.gz")},
	{"TwitterStatus", func() any { return new(twitterRoot) }, mustRead("testdata/twitter_status.json.gz")},
	{"GolangSource", func() any { return new(golangRoot) }, mustRead("testdata/golang_source.json.gz")},
	{"StringUnicode", func() any { return new(stringRoot) }, mustRead("testdata/string_unicode.json.gz")},
}

var arshalers = []struct {
	name          string
	pkgPath       string
	marshal       func(any) ([]byte, error)
	unmarshal     func([]byte, any) error
	marshalWrite  func(io.Writer, any) error
	unmarshalRead func(io.Reader, any) error
}{{
	name:          "JSONv1",
	pkgPath:       "encoding/json",
	marshal:       jsonv1.Marshal,
	unmarshal:     jsonv1.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return jsonv1.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return jsonv1.NewDecoder(r).Decode(v) },
}, {
	name:          "JSONv1in2",
	pkgPath:       "github.com/go-json-experiment/json/v1",
	marshal:       jsonv1in2.Marshal,
	unmarshal:     jsonv1in2.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return jsonv1in2.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return jsonv1in2.NewDecoder(r).Decode(v) },
}, {
	name:          "JSONv2",
	pkgPath:       "github.com/go-json-experiment/json",
	marshal:       func(v any) ([]byte, error) { return jsonv2.Marshal(v) },
	unmarshal:     func(b []byte, v any) error { return jsonv2.Unmarshal(b, v) },
	marshalWrite:  func(w io.Writer, v any) error { return jsonv2.MarshalWrite(w, v) },
	unmarshalRead: func(r io.Reader, v any) error { return jsonv2.UnmarshalRead(r, v) },
}, {
	name:          "JSONIterator",
	pkgPath:       "github.com/json-iterator/go",
	marshal:       jsoniter.Marshal,
	unmarshal:     jsoniter.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return jsoniter.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return jsoniter.NewDecoder(r).Decode(v) },
}, {
	name:          "SegmentJSON",
	pkgPath:       "github.com/segmentio/encoding/json",
	marshal:       segjson.Marshal,
	unmarshal:     segjson.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return segjson.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return segjson.NewDecoder(r).Decode(v) },
}, {
	name:          "GoJSON",
	pkgPath:       "github.com/goccy/go-json",
	marshal:       gojson.Marshal,
	unmarshal:     gojson.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return gojson.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return gojson.NewDecoder(r).Decode(v) },
}, {
	name:          "SonicJSON",
	pkgPath:       "github.com/bytedance/sonic",
	marshal:       sonicjson.Marshal,
	unmarshal:     sonicjson.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return sonicenc.NewStreamEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return sonicdec.NewStreamDecoder(r).Decode(v) },
}, {
	name:          "SonnetJSON",
	pkgPath:       "github.com/sugawarayuuta/sonnet",
	marshal:       sonnetjson.Marshal,
	unmarshal:     sonnetjson.Unmarshal,
	marshalWrite:  func(w io.Writer, v any) error { return sonnetjson.NewEncoder(w).Encode(v) },
	unmarshalRead: func(r io.Reader, v any) error { return sonnetjson.NewDecoder(r).Decode(v) },
}}

func TestRoundtrip(t *testing.T) {
	for _, td := range testdata {
		td := td

		types := []struct {
			name string
			new  func() any
		}{
			{"Concrete", td.new},
			{"Interface", func() any { return new(any) }},
			{"RawValue", func() any { return new(jsontext.Value) }},
		}
		for _, tt := range types {
			tt := tt

			// Use v1 as the reference point for correctness.
			wantVal := tt.new()
			must.Do(jsonv1.Unmarshal(td.data, wantVal))

			// Check all other arshal implementation with respect to v1.
			for _, a := range arshalers {
				a := a
				if a.name == "V1" {
					continue // no need to test v1 with itself
				}

				for _, name := range []string{"Marshal", "MarshalWrite"} {
					t.Run(fmt.Sprintf("%s/%s/%s/%s", td.name, tt.name, a.name, name), func(t *testing.T) {
						t.Parallel()

						var gotBuf []byte
						switch name {
						case "Marshal":
							gotBuf = must.Get(a.marshal(wantVal))
						case "MarshalWrite":
							bb := new(bytes.Buffer)
							must.Do(a.marshalWrite(bb, wantVal))
							gotBuf = bb.Bytes()
						}

						// Checking the marshaled output is tricky.
						// Unmarshal it and verify it matches the result
						// obtained from unmarshaling using v1.
						gotVal := tt.new()
						must.Do(jsonv1.Unmarshal(gotBuf, gotVal))
						if !reflect.DeepEqual(gotVal, wantVal) {
							if diff := cmp.Diff(gotVal, wantVal,
								cmpopts.EquateEmpty(),
								cmp.Comparer(equalRawValue),
							); diff != "" {
								t.Fatalf("mismatch (-got +want):\n%s", diff)
							}
						}
					})
				}

				for _, name := range []string{"Unmarshal", "UnmarshalRead"} {
					t.Run(fmt.Sprintf("%s/%s/%s/%s", td.name, tt.name, a.name, name), func(t *testing.T) {
						t.Parallel()

						gotVal := tt.new()
						switch name {
						case "Unmarshal":
							must.Do(a.unmarshal(td.data, gotVal))
						case "UnmarshalRead":
							must.Do(a.unmarshalRead(bytes.NewReader(td.data), gotVal))
						}

						if !reflect.DeepEqual(gotVal, wantVal) {
							if diff := cmp.Diff(gotVal, wantVal,
								cmp.Comparer(equalRawValue),
							); diff != "" {
								t.Fatalf("mismatch (-got +want):\n%s", diff)
							}
						}
					})
				}
			}
		}
	}
}

// TestStreaming tests whether the implementation is truly streaming,
// meaning that encoding and decoding should not allocate any buffers
// as large as the entire JSON value.
func TestStreaming(t *testing.T) {
	wantStreaming := map[string]bool{
		"JSONv1/Marshal":         false,
		"JSONv1/Unmarshal":       false,
		"JSONv1in2/Marshal":      false,
		"JSONv1in2/Unmarshal":    false,
		"JSONv2/Marshal":         true,
		"JSONv2/Unmarshal":       true,
		"JSONIterator/Marshal":   false,
		"JSONIterator/Unmarshal": true,
		"SegmentJSON/Marshal":    false,
		"SegmentJSON/Unmarshal":  false,
		"GoJSON/Marshal":         false,
		"GoJSON/Unmarshal":       false,
		"SonicJSON/Marshal":      false,
		"SonicJSON/Unmarshal":    false,
		"SonnetJSON/Marshal":     false,
		"SonnetJSON/Unmarshal":   false,
	}

	const size = 1e6
	value := "[" + strings.TrimSuffix(strings.Repeat("{},", size), ",") + "]"
	for _, a := range arshalers {
		for _, funcName := range []string{"Marshal", "Unmarshal"} {
			name := fmt.Sprintf("%s/%s", a.name, funcName)
			t.Run(name, func(t *testing.T) {
				// Run GC multiple times to fully clear any sync.Pools.
				for i := 0; i < 10; i++ {
					runtime.GC()
				}

				// Measure allocations beforehand.
				var statsBefore runtime.MemStats
				runtime.ReadMemStats(&statsBefore)

				// Streaming marshal/unmarshal a large JSON array.
				switch funcName {
				case "Marshal":
					in := make([]struct{}, size)
					out := io.Discard
					must.Do(a.marshalWrite(out, &in))
				case "Unmarshal":
					in := strings.NewReader(value)
					out := make([]struct{}, 0, size)
					must.Do(a.unmarshalRead(in, &out))
				}

				// Measure allocations afterwards.
				var statsAfter runtime.MemStats
				runtime.ReadMemStats(&statsAfter)

				// True streaming implementations only use a small fixed buffer.
				allocBytes := statsAfter.TotalAlloc - statsBefore.TotalAlloc
				allocObjects := statsAfter.Mallocs - statsBefore.Mallocs
				gotStreaming := allocBytes < 1<<16
				if gotStreaming != wantStreaming[name] {
					t.Errorf("streaming = %v, want %v", gotStreaming, wantStreaming[a.name])
				}
				t.Logf("%d bytes allocated, %d objects allocted", allocBytes, allocObjects)
			})
		}
	}
}

// Per RFC 8259, section 8.1, JSON text must be encoded using UTF-8.
func TestValidateUTF8(t *testing.T) {
	type mode string
	const (
		ignored  mode = "ignored"  // invalid UTF-8 is ignored
		replaced mode = "replaced" // invalid UTF-8 is replaced with utf8.RuneError
		rejected mode = "rejected" // invalid UTF-8 is rejected
	)
	wantModes := map[string]mode{
		"JSONv1/Marshal":         replaced,
		"JSONv1/Unmarshal":       replaced,
		"JSONv1in2/Marshal":      replaced,
		"JSONv1in2/Unmarshal":    replaced,
		"JSONv2/Marshal":         rejected,
		"JSONv2/Unmarshal":       rejected,
		"JSONIterator/Marshal":   replaced,
		"JSONIterator/Unmarshal": ignored,
		"SegmentJSON/Marshal":    replaced,
		"SegmentJSON/Unmarshal":  replaced,
		"GoJSON/Marshal":         replaced,
		"GoJSON/Unmarshal":       ignored,
		"SonicJSON/Marshal":      ignored,
		"SonicJSON/Unmarshal":    ignored,
		"SonnetJSON/Marshal":     replaced,
		"SonnetJSON/Unmarshal":   replaced,
	}
	for _, a := range arshalers {
		t.Run(a.name+"/Marshal", func(t *testing.T) {
			var got mode
			switch b, err := a.marshal("\xbe\xef\xff"); {
			case err == nil && string(b) == "\"\xbe\xef\xff\"":
				got = ignored
			case err == nil && string(b) == `"\ufffd\ufffd\ufffd"`:
				got = replaced
			case err != nil:
				got = rejected
			default:
				t.Errorf("unknown mode: json.Marshal = (%s, %v)", b, err)
			}
			if want := wantModes[a.name+"/Marshal"]; got != want {
				t.Errorf("mode = %s, want %s", got, want)
			}
		})
		t.Run(a.name+"/Unmarshal", func(t *testing.T) {
			var got mode
			var s string
			switch err := a.unmarshal([]byte("\"\xbe\xef\xff\""), &s); {
			case err == nil && s == "\xbe\xef\xff":
				got = ignored
			case err == nil && s == "\ufffd\ufffd\ufffd":
				got = replaced
			case err != nil:
				got = rejected
			default:
				t.Errorf("unknown mode: json.Unmarshal = (%q, %v)", s, err)
			}
			if want := wantModes[a.name+"/Unmarshal"]; got != want {
				t.Errorf("mode = %s, want %s", got, want)
			}
		})
	}
}

type duplicateText int

func (duplicateText) MarshalText() ([]byte, error) {
	return []byte("duplicate"), nil
}

// RFC 8259 leaves handling of duplicate JSON object names as undefined.
// RFC 7493 forbids the presence of duplicate JSON object names.
func TestDuplicateNames(t *testing.T) {
	wantAllowDuplicates := map[string]bool{
		"JSONv1":       true,
		"JSONv1in2":    true,
		"JSONv2":       false,
		"JSONIterator": true,
		"SegmentJSON":  true,
		"GoJSON":       true,
		"SonicJSON":    true,
		"SonnetJSON":   true,
	}
	for _, a := range arshalers {
		t.Run(a.name+"/Marshal", func(t *testing.T) {
			_, err := a.marshal(map[duplicateText]int{0: 0, 1: 1})
			gotAllowDuplicates := err == nil
			if gotAllowDuplicates != wantAllowDuplicates[a.name] {
				t.Errorf("AllowDuplicates = %v, want %v", gotAllowDuplicates, wantAllowDuplicates[a.name])
			}
		})
	}
	for _, a := range arshalers {
		t.Run(a.name+"/Unmarshal", func(t *testing.T) {
			var out map[string]int
			err := a.unmarshal([]byte(`{"duplicate":0,"duplicate":1}`), &out)
			gotAllowDuplicates := err == nil
			if gotAllowDuplicates != wantAllowDuplicates[a.name] {
				t.Errorf("AllowDuplicates = %v, want %v", gotAllowDuplicates, wantAllowDuplicates[a.name])
			}
		})
	}
}

var updateParseSuiteResults = flag.Bool("update-parse-suite-results", false, "update the results from running the parsing test suite")

// TestParseSuite tests each JSON implementation against a suite of tests
// that it correctly parses or rejects a given JSON input.
// This test suite comes from "Parsing JSON is a Minefield 💣".
// See https://seriot.ch/projects/parsing_json.html.
func TestParseSuite(t *testing.T) {
	type results struct {
		GotPassingWantFailing map[string][]string
		GotFailingWantPassing map[string][]string
		GotPassingWantEither  map[string][]string
		GotFailingWantEither  map[string][]string
	}

	const dir = "testdata/JSONTestSuite"
	entries := must.Get(os.ReadDir(dir))
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	gotResults := results{make(map[string][]string), make(map[string][]string), make(map[string][]string), make(map[string][]string)}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || name == "results.json" {
			continue
		}
		b := must.Get(os.ReadFile(filepath.Join(dir, name)))
		name = strings.TrimSuffix(name, ".json")
		for _, a := range arshalers {
			prefix, suffix, _ := strings.Cut(name, "_")
			switch err := a.unmarshal(b, new(jsontext.Value)); {
			case prefix == "n" && err == nil:
				gotResults.GotPassingWantFailing[suffix] = append(gotResults.GotPassingWantFailing[suffix], a.name)
			case prefix == "y" && err != nil:
				gotResults.GotFailingWantPassing[suffix] = append(gotResults.GotFailingWantPassing[suffix], a.name)
			case prefix == "i" && err == nil:
				gotResults.GotPassingWantEither[suffix] = append(gotResults.GotPassingWantEither[suffix], a.name)
			case prefix == "i" && err != nil:
				gotResults.GotFailingWantEither[suffix] = append(gotResults.GotFailingWantEither[suffix], a.name)
			}
		}
	}

	if *updateParseSuiteResults {
		b := must.Get(jsonv1.Marshal(gotResults))
		b = append(bytes.TrimSuffix(b, []byte("}")), "\n}"...) // formatting hint for hujson.Format
		b, _ = hujson.Format(b)
		must.Do(os.WriteFile(filepath.Join(dir, "results.json"), b, 0664))
	} else {
		want := must.Get(os.ReadFile(filepath.Join(dir, "results.json")))
		var wantResults results
		must.Do(jsonv1.Unmarshal(want, &wantResults))
		if diff := cmp.Diff(gotResults, wantResults); diff != "" {
			t.Fatalf("mismatch (-got +want):\n%s", diff)
		}
	}
}

// The output of a MarshalJSON method should be validated.
func TestValidateMarshalJSON(t *testing.T) {
	type mode string
	const (
		ignored  mode = "ignored"  // invalid MarshalJSON output ignored
		rejected mode = "rejected" // invalid MarshalJSON output rejected
	)
	wantModes := map[string]mode{
		"JSONv1":       rejected,
		"JSONv1in2":    rejected,
		"JSONv2":       rejected,
		"JSONIterator": ignored,
		"SegmentJSON":  rejected,
		"GoJSON":       rejected,
		"SonicJSON":    rejected,
		"SonnetJSON":   rejected,
	}
	for _, a := range arshalers {
		t.Run(a.name, func(t *testing.T) {
			var got mode
			if _, err := a.marshal(jsontext.Value("<junk>")); err == nil {
				got = ignored
			} else {
				got = rejected
			}
			if want := wantModes[a.name]; got != want {
				t.Errorf("mode = %s, want %s", got, want)
			}
		})
	}
}

// JSON specification does not specify any ordering for JSON object members.
// Sorting the order is convenient, but is a performance cost.
func TestMapDeterminism(t *testing.T) {
	wantDeterministic := map[string]bool{
		"JSONv1":       true,
		"JSONv1in2":    true,
		"JSONv2":       false,
		"JSONIterator": false,
		"SegmentJSON":  true,
		"GoJSON":       true,
		"SonicJSON":    false,
		"SonnetJSON":   false,
	}
	for _, a := range arshalers {
		t.Run(a.name, func(t *testing.T) {
			const iterations = 10
			in := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7, 8: 8, 9: 9}
			outs := make(map[string]bool)
			for i := 0; i < iterations; i++ {
				b, err := a.marshal(in)
				if err != nil {
					t.Fatalf("json.Marshal error: %v", err)
				}
				outs[string(b)] = true
			}
			gotDeterministic := len(outs) == 1
			wantDeterministic := wantDeterministic[a.name]
			switch {
			case gotDeterministic && !wantDeterministic:
				t.Log("deterministic = true, want false")
			case !gotDeterministic && wantDeterministic:
				t.Error("deterministic = false, want true")
			}
		})
	}
}

// Implementations differ regarding how much of the output value is modified
// when an unmarshaling error is encountered.
//
// There are generally two reasonable behaviors:
//  1. Make no mutating changes to the output if the input is invalid.
//  2. Make as many changes as possible up until the input becomes invalid.
func TestUnmarshalErrors(t *testing.T) {
	type Struct struct{ A, B, C []int }
	want := map[string]Struct{
		"JSONv1":       {},                            // none
		"JSONv1in2":    {},                            // none
		"JSONv2":       {A: []int{1}, B: []int{2, 0}}, // all
		"JSONIterator": {A: []int{1}, B: []int{2, 0}}, // all
		"SegmentJSON":  {A: []int{1}, B: []int{2}},    // some
		"GoJSON":       {A: []int{1}},                 // some
		"SonicJSON":    {A: []int{1}, B: []int{2, 0}}, // all
		"SonnetJSON":   {A: []int{1}},                 // some
	}
	for _, a := range arshalers {
		t.Run(a.name, func(t *testing.T) {
			var out Struct
			err := a.unmarshal([]byte(`{"A":[1],"B":[2,invalid`), &out)
			if err == nil {
				t.Errorf("json.Unmarshal error is nil, want non-nil")
			}
			if !reflect.DeepEqual(out, want[a.name]) {
				t.Errorf("json.Unmarshal = %v, want %v", out, want[a.name])
			}
		})
	}
}

var checkBinarySize = flag.Bool("check-binary-size", false, "check binary sizes of each JSON implementation")

func TestBinarySize(t *testing.T) {
	if !*checkBinarySize {
		t.Skip("--check-binary-size is not specified")
	}
	dir := must.Get(os.MkdirTemp(must.Get(os.Getwd()), "binsize"))
	defer os.RemoveAll(dir)
	t.Logf("GOOS:%s GOARCH:%s", runtime.GOOS, runtime.GOARCH)
	for _, a := range arshalers {
		t.Run(a.name, func(t *testing.T) {
			var bb bytes.Buffer
			bb.WriteString("package main\n")
			bb.WriteString("import json " + strconv.Quote(a.pkgPath) + "\n")
			bb.WriteString("var v any\n")
			bb.WriteString("func main() {\n")
			bb.WriteString("v, v = json.Marshal(v)\n")
			bb.WriteString("v = json.Unmarshal(v.([]byte), v)\n")
			bb.WriteString("}\n")
			must.Do(os.WriteFile(filepath.Join(dir, "main.go"), bb.Bytes(), 0664))

			cmd := exec.Command("go", "build", "main.go")
			cmd.Dir = dir
			must.Do(cmd.Run())

			t.Logf("size: %0.3f MiB", float64(must.Get(os.Stat(filepath.Join(dir, "main"))).Size())/(1<<20))
		})
	}
}

func Benchmark(b *testing.B) {
	for _, td := range testdata {
		types := []struct {
			name string
			new  func() any
		}{
			{"Concrete", td.new},
			{"Interface", func() any { return new(any) }},
			{"RawValue", func() any { return new(jsontext.Value) }},
		}
		for _, tt := range types {
			for _, a := range arshalers {
				val := tt.new()
				must.Do(a.unmarshal(td.data, val))
				b.Run(fmt.Sprintf("%s/%s/%s/Marshal", td.name, tt.name, a.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						must.Get(a.marshal(val))
					}
				})
				b.Run(fmt.Sprintf("%s/%s/%s/Unmarshal", td.name, tt.name, a.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						must.Do(a.unmarshal(td.data, tt.new()))
					}
				})
			}
		}
	}
}
