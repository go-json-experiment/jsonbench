// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// This program processes the benchmark output and
// outputs a series of tab-separated tables.
package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

func appendIfNotExist[T comparable](vs []T, v T) []T {
	for i := 0; i < len(vs); i++ {
		if vs[i] == v {
			return vs
		}
	}
	return append(vs, v)
}

func main() {
	// Read the benchmark output.
	files := []string{"results.log"}
	if len(os.Args) > 1 {
		files = os.Args[1:]
	}
	var lines []string
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			panic(err)
		}
		lines = append(lines, strings.Split(string(b), "\n")...)
	}

	var tests, types, impls, funcs []string
	runtimes := make(map[string]metric)
	allocBytes := make(map[string]metric)
	numAllocs := make(map[string]metric)
	metrics := []struct {
		name    string
		metrics map[string]metric
	}{
		{"Runtimes", runtimes},
		{"AllocBytes", allocBytes},
		{"NumAllocs", numAllocs},
	}

	// Parse the benchmark output.
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 || !strings.HasPrefix(fields[0], "Benchmark/") {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(fields[0]), "01245789"), "-"), "Benchmark/")
		segments := strings.Split(name, "/")
		if len(segments) != 4 {
			continue
		}
		tests = appendIfNotExist(tests, segments[0])
		types = appendIfNotExist(types, segments[1])
		impls = appendIfNotExist(impls, segments[2])
		funcs = appendIfNotExist(funcs, segments[3])
		for _, field := range fields[1:] {
			field = strings.TrimSpace(field)
			switch {
			case strings.HasSuffix(field, " ns/op"):
				if n, err := strconv.ParseInt(strings.TrimSuffix(field, " ns/op"), 10, 64); err == nil {
					runtimes[name] = runtimes[name].Add(n)
				}
			case strings.HasSuffix(field, " B/op"):
				if n, err := strconv.ParseInt(strings.TrimSuffix(field, " B/op"), 10, 64); err == nil {
					allocBytes[name] = allocBytes[name].Add(n)
				}
			case strings.HasSuffix(field, " allocs/op"):
				if n, err := strconv.ParseInt(strings.TrimSuffix(field, " allocs/op"), 10, 64); err == nil {
					numAllocs[name] = numAllocs[name].Add(n)
				}
			}
		}
	}

	// Output tab-separated tables for all the results.
	for _, met := range metrics {
		for _, fun := range funcs {
			for _, typ := range types {
				fmt.Printf("%s/%s/%s", met.name, fun, typ)
				for _, imp := range impls {
					fmt.Printf("\t%s", imp)
				}
				fmt.Println()
				for _, td := range tests {
					fmt.Printf("%s", td)
					var m0 float64
					for i, imp := range impls {
						name := fmt.Sprintf("%s/%s/%s/%s", td, typ, imp, fun)
						m := met.metrics[name].Mean()
						if i == 0 {
							m0 = m
							m = 1.0
						} else {
							m = m / m0
						}
						fmt.Printf("\t%0.6f", m)
					}
					fmt.Println()
				}
				fmt.Println()
			}
		}
	}
}

type metric []int64

func (r metric) Add(n int64) metric {
	return append(r, n)
}
func (r metric) Mean() float64 {
	var sum float64
	for _, n := range r {
		sum += float64(n)
	}
	return sum / float64(len(r))
}
func (r metric) Median() float64 {
	r = append(metric(nil), r...)
	sort.Slice(r, func(i, j int) bool { return r[i] < r[j] })
	if len(r) > 0 {
		return float64(r[len(r)/2])
	}
	return math.NaN()
}
