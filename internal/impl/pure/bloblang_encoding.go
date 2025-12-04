// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"github.com/redpanda-data/benthos/v4/internal/bloblang/query"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
)

func init() {
	bloblang.MustRegisterMethodV2("compress",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryEncoding).
			Description(`Compresses a string or byte array using the specified compression algorithm. Returns compressed data as bytes. Useful for reducing payload size before transmission or storage.`).
			Param(bloblang.NewStringParam("algorithm").Description("The compression algorithm: `flate`, `gzip`, `pgzip` (parallel gzip), `lz4`, `snappy`, `zlib`, or `zstd`.")).
			Param(bloblang.NewInt64Param("level").Description("Compression level (default: -1 for default compression). Higher values increase compression ratio but use more CPU. Range and effect varies by algorithm.").Default(-1)).
			Example("Compress and encode for safe transmission", `root.compressed = content().bytes().compress("gzip").encode("base64")`,
				[2]string{
					`{"message":"hello world I love space"}`,
					`{"compressed":"H4sIAAAJbogA/wAmANn/eyJtZXNzYWdlIjoiaGVsbG8gd29ybGQgSSBsb3ZlIHNwYWNlIn0DAHEvdwomAAAA"}`,
				},
			).
			Example("Compare compression ratios across algorithms", `root.original_size = content().length()
root.gzip_size = content().compress("gzip").length()
root.lz4_size = content().compress("lz4").length()`,
				[2]string{
					`The quick brown fox jumps over the lazy dog. The quick brown fox jumps over the lazy dog.`,
					`{"gzip_size":114,"lz4_size":85,"original_size":89}`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			level, err := args.GetInt64("level")
			if err != nil {
				return nil, err
			}
			algStr, err := args.GetString("algorithm")
			if err != nil {
				return nil, err
			}
			algFn, err := strToCompressFunc(algStr)
			if err != nil {
				return nil, err
			}
			return bloblang.BytesMethod(func(data []byte) (any, error) {
				return algFn(int(level), data)
			}), nil
		})

	bloblang.MustRegisterMethodV2("decompress",
		bloblang.NewPluginSpec().
			Category(query.MethodCategoryEncoding).
			Description(`Decompresses a byte array using the specified decompression algorithm. Returns decompressed data as bytes. Use with data that was previously compressed using the corresponding algorithm.`).
			Param(bloblang.NewStringParam("algorithm").Description("The decompression algorithm: `gzip`, `pgzip` (parallel gzip), `zlib`, `bzip2`, `flate`, `snappy`, `lz4`, or `zstd`.")).
			Example("Decompress base64-encoded compressed data", `root = this.compressed.decode("base64").decompress("gzip")`,
				[2]string{
					`{"compressed":"H4sIAN12MWkAA8tIzcnJVyjPL8pJUfBUyMkvS1UoLkhMTgUAQpDxbxgAAAA="}`,
					`hello world I love space`,
				},
			).
			Example("Convert decompressed bytes to string for JSON output", `root.message = this.compressed.decode("base64").decompress("gzip").string()`,
				[2]string{
					`{"compressed":"H4sIAN12MWkAA8tIzcnJVyjPL8pJUfBUyMkvS1UoLkhMTgUAQpDxbxgAAAA="}`,
					`{"message":"hello world I love space"}`,
				},
			),
		func(args *bloblang.ParsedParams) (bloblang.Method, error) {
			algStr, err := args.GetString("algorithm")
			if err != nil {
				return nil, err
			}
			algFn, err := strToDecompressFunc(algStr)
			if err != nil {
				return nil, err
			}
			return bloblang.BytesMethod(func(data []byte) (any, error) {
				return algFn(data)
			}), nil
		})
}
