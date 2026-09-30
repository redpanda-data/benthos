// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"hash/crc32"
	"hash/fnv"
	"net/url"
	"strconv"

	"github.com/OneOfOne/xxhash"
	"github.com/gofrs/uuid/v5"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 encoding-adjacent methods that are deterministic given their
// inputs (no env access, no time, no randomness). See PARITY.md.

func init() {
	bloblangv2.MustRegisterMethod("hash",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description("Hashes a string or bytes using the named algorithm and returns the digest as bytes. Available algorithms: hmac_sha1, hmac_sha256, hmac_sha512, md5, sha1, sha256, sha512, xxhash64, crc32, fnv32. The hmac_* algorithms require the key argument; crc32 supports an optional polynomial.").
			Param(bloblangv2.NewStringParam("algorithm").Description("The hashing algorithm to use.")).
			Param(bloblangv2.NewStringParam("key").Description("Key for HMAC variants.").Default("")).
			Param(bloblangv2.NewStringParam("polynomial").Description(`crc32 polynomial: "IEEE", "Castagnoli", or "Koopman".`).Default("IEEE")),
		hashV2Ctor,
	)

	bloblangv2.MustRegisterMethod("uuid_v5",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description(`Returns a deterministic UUID v5 derived from the receiver string and a namespace. The namespace may be one of "dns", "url", "oid", "x500", or any RFC-9562 UUID. Empty / unset uses the nil namespace.`).
			Param(bloblangv2.NewStringParam("ns").Description("Namespace name or UUID.").Default("")),
		uuidV5V2Ctor,
	)

	bloblangv2.MustRegisterMethod("compress",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description("Compresses the receiver bytes using the named algorithm and returns the compressed bytes. Supported algorithms: flate, gzip, pgzip, lz4, snappy, zlib, zstd.").
			Param(bloblangv2.NewStringParam("algorithm").Description("The compression algorithm.")).
			Param(bloblangv2.NewInt64Param("level").Description("Compression level (-1 selects the algorithm default).").Default(int64(-1))),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			algStr, err := args.GetString("algorithm")
			if err != nil {
				return nil, err
			}
			level, err := args.GetInt64("level")
			if err != nil {
				return nil, err
			}
			algFn, err := strToCompressFunc(algStr)
			if err != nil {
				return nil, err
			}
			return bloblangv2.BytesMethod(func(data []byte) (any, error) {
				return algFn(int(level), data)
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("decompress",
		bloblangv2.NewPluginSpec().
			Category("Encoding").
			Description("Decompresses the receiver bytes using the named algorithm and returns the decompressed bytes. Supported algorithms: gzip, pgzip, zlib, bzip2, flate, snappy, lz4, zstd.").
			Param(bloblangv2.NewStringParam("algorithm").Description("The decompression algorithm.")),
		func(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			algStr, err := args.GetString("algorithm")
			if err != nil {
				return nil, err
			}
			algFn, err := strToDecompressFunc(algStr)
			if err != nil {
				return nil, err
			}
			return bloblangv2.BytesMethod(func(data []byte) (any, error) {
				return algFn(data)
			}), nil
		},
	)

	bloblangv2.MustRegisterMethod("parse_form_url_encoded",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Parses a url-encoded query string (e.g. an x-www-form-urlencoded request body) and returns an object. Repeated keys are surfaced as arrays."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return bloblangv2.StringMethod(func(s string) (any, error) {
				values, err := url.ParseQuery(s)
				if err != nil {
					return nil, fmt.Errorf("failed to parse value as url-encoded data: %w", err)
				}
				return urlValuesToMap(values), nil
			}), nil
		},
	)
}

func hashV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	algo, err := args.GetString("algorithm")
	if err != nil {
		return nil, err
	}
	key, err := args.GetString("key")
	if err != nil {
		return nil, err
	}
	poly, err := args.GetString("polynomial")
	if err != nil {
		return nil, err
	}

	hashFn, err := buildHashFn(algo, []byte(key), poly)
	if err != nil {
		return nil, err
	}
	return func(v any) (any, error) {
		switch t := v.(type) {
		case string:
			return hashFn([]byte(t))
		case []byte:
			return hashFn(t)
		}
		return nil, fmt.Errorf("expected string or bytes receiver, got %T", v)
	}, nil
}

func buildHashFn(algo string, key []byte, poly string) (func([]byte) ([]byte, error), error) {
	requireKey := func() error {
		if len(key) == 0 {
			return fmt.Errorf("hash algorithm %v requires a key argument", algo)
		}
		return nil
	}
	switch algo {
	case "hmac_sha1", "hmac-sha1":
		if err := requireKey(); err != nil {
			return nil, err
		}
		return func(b []byte) ([]byte, error) {
			h := hmac.New(sha1.New, key)
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "hmac_sha256", "hmac-sha256":
		if err := requireKey(); err != nil {
			return nil, err
		}
		return func(b []byte) ([]byte, error) {
			h := hmac.New(sha256.New, key)
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "hmac_sha512", "hmac-sha512":
		if err := requireKey(); err != nil {
			return nil, err
		}
		return func(b []byte) ([]byte, error) {
			h := hmac.New(sha512.New, key)
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "md5":
		return func(b []byte) ([]byte, error) {
			h := md5.New()
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "sha1":
		return func(b []byte) ([]byte, error) {
			h := sha1.New()
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "sha256":
		return func(b []byte) ([]byte, error) {
			h := sha256.New()
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "sha512":
		return func(b []byte) ([]byte, error) {
			h := sha512.New()
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "xxhash64":
		return func(b []byte) ([]byte, error) {
			h := xxhash.New64()
			_, _ = h.Write(b)
			return []byte(strconv.FormatUint(h.Sum64(), 10)), nil
		}, nil
	case "crc32":
		return func(b []byte) ([]byte, error) {
			var h hash.Hash
			switch poly {
			case "IEEE":
				h = crc32.NewIEEE()
			case "Castagnoli":
				h = crc32.New(crc32.MakeTable(crc32.Castagnoli))
			case "Koopman":
				h = crc32.New(crc32.MakeTable(crc32.Koopman))
			default:
				return nil, fmt.Errorf("unsupported crc32 polynomial %q", poly)
			}
			_, _ = h.Write(b)
			return h.Sum(nil), nil
		}, nil
	case "fnv32":
		return func(b []byte) ([]byte, error) {
			h := fnv.New32()
			_, _ = h.Write(b)
			return []byte(strconv.FormatUint(uint64(h.Sum32()), 10)), nil
		}, nil
	}
	return nil, fmt.Errorf("unrecognized hash type: %v", algo)
}

func uuidV5V2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	ns, err := args.GetString("ns")
	if err != nil {
		return nil, err
	}
	var nsUUID uuid.UUID
	switch ns {
	case "":
		nsUUID = uuid.Nil
	case "dns", "DNS":
		nsUUID = uuid.NamespaceDNS
	case "url", "URL":
		nsUUID = uuid.NamespaceURL
	case "oid", "OID":
		nsUUID = uuid.NamespaceOID
	case "x500", "X500":
		nsUUID = uuid.NamespaceX500
	default:
		nsUUID, err = uuid.FromString(ns)
		if err != nil {
			return nil, fmt.Errorf("invalid ns uuid: %q", ns)
		}
	}
	return bloblangv2.StringMethod(func(s string) (any, error) {
		return uuid.NewV5(nsUUID, s).String(), nil
	}), nil
}
