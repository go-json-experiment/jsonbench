// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonbench

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"testing"

	jsonv1 "encoding/json"

	sonicjson "github.com/bytedance/sonic"
	jsonv2 "github.com/go-json-experiment/json"
	gojson "github.com/goccy/go-json"
	jsoniter "github.com/json-iterator/go"
	segjson "github.com/segmentio/encoding/json"

	"github.com/dsnet/try"
)

func mustRead(path string) []byte {
	b := try.E1(os.ReadFile(path))
	zr := try.E1(gzip.NewReader(bytes.NewReader(b)))
	b = try.E1(io.ReadAll(zr))
	return b
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
	name      string
	marshal   func(any) ([]byte, error)
	unmarshal func([]byte, any) error
}{{
	name:      "JSONv1",
	marshal:   jsonv1.Marshal,
	unmarshal: jsonv1.Unmarshal,
}, {
	name:      "JSONv2",
	marshal:   jsonv2.Marshal,
	unmarshal: jsonv2.Unmarshal,
}, {
	name:      "JSONIterator",
	marshal:   jsoniter.Marshal,
	unmarshal: jsoniter.Unmarshal,
}, {
	name:      "SegmentJSON",
	marshal:   segjson.Marshal,
	unmarshal: segjson.Unmarshal,
}, {
	name:      "GoJSON",
	marshal:   gojson.Marshal,
	unmarshal: gojson.Unmarshal,
}, {
	name:      "SonicJSON",
	marshal:   sonicjson.Marshal,
	unmarshal: sonicjson.Unmarshal,
}}

func Benchmark(b *testing.B) {
	for _, td := range testdata {
		types := []struct {
			name string
			new  func() any
		}{
			{"Concrete", td.new},
			{"Interface", func() any { return new(any) }},
			{"RawValue", func() any { return new(jsonv2.RawValue) }},
		}
		for _, tt := range types {
			for _, a := range arshalers {
				val := tt.new()
				try.E(a.unmarshal(td.data, val))
				b.Run(fmt.Sprintf("%s/%s/%s/Marshal", td.name, tt.name, a.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						try.E1(a.marshal(val))
					}
				})
				b.Run(fmt.Sprintf("%s/%s/%s/Unmarshal", td.name, tt.name, a.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						try.E(a.unmarshal(td.data, tt.new()))
					}
				})
			}
		}
	}
}
