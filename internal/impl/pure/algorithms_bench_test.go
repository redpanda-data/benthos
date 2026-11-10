// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// benchJSONBody returns a deterministic ~4.5 KB JSON document shaped like an
// HTTP enrichment request body, with enough variety to compress realistically.
func benchJSONBody(tb testing.TB) []byte {
	tb.Helper()
	words := strings.Fields(`the a of to and in that is for on with as was at by from
		market company report shares quarter revenue growth analysts said percent year
		government policy minister election vote city police officials court ruling
		technology data platform users launch product service network security cloud
		climate energy prices oil gas power storm weather season team match player coach`)
	rng := rand.New(rand.NewPCG(1, 2))
	sentence := func(n int) string {
		var sb strings.Builder
		for i := range n {
			if i > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(words[rng.IntN(len(words))])
		}
		return sb.String()
	}
	paragraphs := make([]string, 6)
	for i := range paragraphs {
		paragraphs[i] = sentence(80)
	}
	entities := make([]map[string]any, 12)
	for i := range entities {
		entities[i] = map[string]any{
			"name":       sentence(2),
			"type":       []string{"PERSON", "ORG", "LOCATION"}[i%3],
			"salience":   rng.Float64(),
			"mentions":   rng.IntN(20),
			"externalId": fmt.Sprintf("ent-%08x", rng.Uint32()),
		}
	}
	doc := map[string]any{
		"id":          fmt.Sprintf("%016x", rng.Uint64()),
		"url":         "https://news.example.com/2026/10/16/" + strings.ReplaceAll(sentence(6), " ", "-"),
		"title":       sentence(12),
		"lang":        "en",
		"mediaType":   "news",
		"publishedAt": "2026-10-16T09:41:27Z",
		"source":      map[string]any{"id": rng.IntN(1 << 30), "name": sentence(3), "country": "GB"},
		"body":        paragraphs,
		"entities":    entities,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

var benchSink []byte

func BenchmarkCompressFunc(b *testing.B) {
	body := benchJSONBody(b)
	for _, algName := range []string{"gzip", "zlib", "flate"} {
		fn, err := strToCompressFunc(algName)
		if err != nil {
			b.Fatal(err)
		}
		for _, level := range []int{1, 3, 6} {
			b.Run(fmt.Sprintf("alg=%v/level=%v", algName, level), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for b.Loop() {
					out, err := fn(level, body)
					if err != nil {
						b.Fatal(err)
					}
					benchSink = out
				}
			})
		}
	}
}

func BenchmarkDecompressFunc(b *testing.B) {
	body := benchJSONBody(b)
	for _, algName := range []string{"gzip", "zlib", "flate"} {
		cfn, err := strToCompressFunc(algName)
		if err != nil {
			b.Fatal(err)
		}
		dfn, err := strToDecompressFunc(algName)
		if err != nil {
			b.Fatal(err)
		}
		compressed, err := cfn(6, body)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("alg=%v", algName), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				out, err := dfn(compressed)
				if err != nil {
					b.Fatal(err)
				}
				benchSink = out
			}
		})
	}
}

func TestBenchJSONBodySize(t *testing.T) {
	if n := len(benchJSONBody(t)); n < 4000 || n > 5500 {
		t.Fatalf("benchmark body is %d bytes, want ~4-5 KB", n)
	}
}
