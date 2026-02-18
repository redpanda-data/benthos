// Copyright 2025 Redpanda Data, Inc.

package query

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/ascii85"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"hash/fnv"
	"html"
	"io"
	"math"
	"math/bits"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/OneOfOne/xxhash"
	"github.com/gofrs/uuid/v5"
	"github.com/tilinna/z85"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/internal/value"
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"bytes", "",
	).InCategory(
		MethodCategoryCoercion,
		"Marshal a value into a byte array. If the value is already a byte array it is unchanged.",
		NewExampleSpec("",
			`root.first_byte = this.name.bytes().index(0)`,
			`{"name":"foobar bazson"}`,
			`{"first_byte":102}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			return value.IToBytes(v), nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"capitalize", "",
	).InCategory(
		MethodCategoryStrings,
		"Converts the first letter of each word in a string to uppercase (title case). Useful for formatting names, titles, and headings.",
		NewExampleSpec("",
			`root.title = this.title.capitalize()`,
			`{"title":"the foo bar"}`,
			`{"title":"The Foo Bar"}`,
		),
		NewExampleSpec("",
			`root.name = this.name.capitalize()`,
			`{"name":"alice smith"}`,
			`{"name":"Alice Smith"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return cases.Title(language.English).String(t), nil
			case []byte:
				return cases.Title(language.English).Bytes(t), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"encode", "",
	).InCategory(
		MethodCategoryEncoding,
		"Encodes a string or byte array target according to a chosen scheme and returns a string result. Available schemes are: `base64`, `base64url` https://rfc-editor.org/rfc/rfc4648.html[(RFC 4648 with padding characters)], `base64rawurl` https://rfc-editor.org/rfc/rfc4648.html[(RFC 4648 without padding characters)], `hex`, `ascii85`.",
		// NOTE: z85 has been removed from the list until we can support
		// misaligned data automatically. It'll still be supported for backwards
		// compatibility, but given it behaves differently to `ascii85` I think
		// it's a poor user experience to expose it.
		NewExampleSpec("",
			`root.encoded = this.value.encode("hex")`,
			`{"value":"hello world"}`,
			`{"encoded":"68656c6c6f20776f726c64"}`,
		),
		NewExampleSpec("",
			`root.encoded = content().encode("ascii85")`,
			`this is totally unstructured data`,
			"{\"encoded\":\"FD,B0+DGm>FDl80Ci\\\"A>F`)8BEckl6F`M&(+Cno&@/\"}",
		),
	).Param(ParamString("scheme", "The encoding scheme to use.")),
	func(args *ParsedParams) (simpleMethod, error) {
		schemeStr, err := args.FieldString("scheme")
		if err != nil {
			return nil, err
		}

		var schemeFn func([]byte) (string, error)
		switch schemeStr {
		case "base64":
			schemeFn = func(b []byte) (string, error) {
				var buf bytes.Buffer
				e := base64.NewEncoder(base64.StdEncoding, &buf)
				_, _ = e.Write(b)
				e.Close()
				return buf.String(), nil
			}
		case "base64url":
			schemeFn = func(b []byte) (string, error) {
				var buf bytes.Buffer
				e := base64.NewEncoder(base64.URLEncoding, &buf)
				_, _ = e.Write(b)
				e.Close()
				return buf.String(), nil
			}
		case "base64rawurl":
			schemeFn = func(b []byte) (string, error) {
				var buf bytes.Buffer
				e := base64.NewEncoder(base64.RawURLEncoding, &buf)
				_, _ = e.Write(b)
				e.Close()
				return buf.String(), nil
			}
		case "hex":
			schemeFn = func(b []byte) (string, error) {
				var buf bytes.Buffer
				e := hex.NewEncoder(&buf)
				if _, err := e.Write(b); err != nil {
					return "", err
				}
				return buf.String(), nil
			}
		case "ascii85":
			schemeFn = func(b []byte) (string, error) {
				var buf bytes.Buffer
				e := ascii85.NewEncoder(&buf)
				if _, err := e.Write(b); err != nil {
					return "", err
				}
				if err := e.Close(); err != nil {
					return "", err
				}
				return buf.String(), nil
			}
		case "z85":
			schemeFn = func(b []byte) (string, error) {
				// TODO: Update this to support misaligned input data similar to the
				// ascii85 encoder.
				enc := make([]byte, z85.EncodedLen(len(b)))
				if _, err := z85.Encode(enc, b); err != nil {
					return "", err
				}
				return string(enc), nil
			}
		default:
			return nil, fmt.Errorf("unrecognized encoding type: %v", schemeStr)
		}

		return func(v any, ctx FunctionContext) (any, error) {
			var res string
			var err error
			switch t := v.(type) {
			case string:
				res, err = schemeFn([]byte(t))
			case []byte:
				res, err = schemeFn(t)
			default:
				err = value.NewTypeError(v, value.TString)
			}
			return res, err
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"decode", "",
	).InCategory(
		MethodCategoryEncoding,
		"Decodes an encoded string target according to a chosen scheme and returns the result as a byte array. When mapping the result to a JSON field the value should be cast to a string using the method `string`, or encoded using the method `encode`, otherwise it will be base64 encoded by default.\n\nAvailable schemes are: `base64`, `base64url` https://rfc-editor.org/rfc/rfc4648.html[(RFC 4648 with padding characters)], `base64rawurl` https://rfc-editor.org/rfc/rfc4648.html[(RFC 4648 without padding characters)], `hex`, `ascii85`.",
		// NOTE: z85 has been removed from the list until we can support
		// misaligned data automatically. It'll still be supported for backwards
		// compatibility, but given it behaves differently to `ascii85` I think
		// it's a poor user experience to expose it.
		NewExampleSpec("",
			`root.decoded = this.value.decode("hex").string()`,
			`{"value":"68656c6c6f20776f726c64"}`,
			`{"decoded":"hello world"}`,
		),
		NewExampleSpec("",
			`root = this.encoded.decode("ascii85")`,
			"{\"encoded\":\"FD,B0+DGm>FDl80Ci\\\"A>F`)8BEckl6F`M&(+Cno&@/\"}",
			`this is totally unstructured data`,
		),
	).Param(ParamString("scheme", "The decoding scheme to use.")),
	func(args *ParsedParams) (simpleMethod, error) {
		schemeStr, err := args.FieldString("scheme")
		if err != nil {
			return nil, err
		}

		var schemeFn func([]byte) ([]byte, error)
		switch schemeStr {
		case "base64":
			schemeFn = func(b []byte) ([]byte, error) {
				e := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(b))
				return io.ReadAll(e)
			}
		case "base64url":
			schemeFn = func(b []byte) ([]byte, error) {
				e := base64.NewDecoder(base64.URLEncoding, bytes.NewReader(b))
				return io.ReadAll(e)
			}
		case "base64rawurl":
			schemeFn = func(b []byte) ([]byte, error) {
				e := base64.NewDecoder(base64.RawURLEncoding, bytes.NewReader(b))
				return io.ReadAll(e)
			}
		case "hex":
			schemeFn = func(b []byte) ([]byte, error) {
				e := hex.NewDecoder(bytes.NewReader(b))
				return io.ReadAll(e)
			}
		case "ascii85":
			schemeFn = func(b []byte) ([]byte, error) {
				e := ascii85.NewDecoder(bytes.NewReader(b))
				return io.ReadAll(e)
			}
		case "z85":
			schemeFn = func(b []byte) ([]byte, error) {
				// TODO: Update this to support misaligned input data similar to the
				// ascii85 decoder.
				dec := make([]byte, z85.DecodedLen(len(b)))
				if _, err := z85.Decode(dec, b); err != nil {
					return nil, err
				}
				return dec, nil
			}
		default:
			return nil, fmt.Errorf("unrecognized encoding type: %v", schemeStr)
		}

		return func(v any, ctx FunctionContext) (any, error) {
			var res []byte
			var err error
			switch t := v.(type) {
			case string:
				res, err = schemeFn([]byte(t))
			case []byte:
				res, err = schemeFn(t)
			default:
				err = value.NewTypeError(v, value.TString)
			}
			return res, err
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"encrypt_aes", "",
	).InCategory(
		MethodCategoryEncoding,
		"Encrypts a string or byte array target according to a chosen AES encryption method and returns a string result. The algorithms require a key and an initialization vector / nonce. Available schemes are: `ctr`, `gcm`, `ofb`, `cbc`.",
		NewExampleSpec("",
			`let key = "2b7e151628aed2a6abf7158809cf4f3c".decode("hex")
let vector = "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff".decode("hex")
root.encrypted = this.value.encrypt_aes("ctr", $key, $vector).encode("hex")`,
			`{"value":"hello world!"}`,
			`{"encrypted":"84e9b31ff7400bdf80be7254"}`,
		),
	).
		Param(ParamString("scheme", "The scheme to use for encryption, one of `ctr`, `gcm`, `ofb`, `cbc`.")).
		Param(ParamString("key", "A key to encrypt with.")).
		Param(ParamString("iv", "An initialization vector / nonce.")),
	func(args *ParsedParams) (simpleMethod, error) {
		schemeStr, err := args.FieldString("scheme")
		if err != nil {
			return nil, err
		}
		keyStr, err := args.FieldString("key")
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher([]byte(keyStr))
		if err != nil {
			return nil, err
		}

		ivStr, err := args.FieldString("iv")
		if err != nil {
			return nil, err
		}
		iv := []byte(ivStr)

		switch schemeStr {
		case "ctr":
			fallthrough
		case "ofb":
			fallthrough
		case "cbc":
			if len(iv) != block.BlockSize() {
				return nil, errors.New("the key must match the initialisation vector size")
			}
		}

		var schemeFn func([]byte) (string, error)
		switch schemeStr {
		case "ctr":
			schemeFn = func(b []byte) (string, error) {
				ciphertext := make([]byte, len(b))
				stream := cipher.NewCTR(block, iv)
				stream.XORKeyStream(ciphertext, b)
				return string(ciphertext), nil
			}
		case "gcm":
			schemeFn = func(b []byte) (string, error) {
				ciphertext := make([]byte, 0, len(b))
				stream, err := cipher.NewGCM(block)
				if err != nil {
					return "", fmt.Errorf("creating gcm failed: %w", err)
				}
				ciphertext = stream.Seal(ciphertext, iv, b, nil)
				return string(ciphertext), nil
			}
		case "ofb":
			schemeFn = func(b []byte) (string, error) {
				ciphertext := make([]byte, len(b))
				//nolint:staticcheck // SA1019: cipher.NewOFB has been deprecated since Go 1.24
				stream := cipher.NewOFB(block, iv)
				stream.XORKeyStream(ciphertext, b)
				return string(ciphertext), nil
			}
		case "cbc":
			schemeFn = func(b []byte) (string, error) {
				if len(b)%aes.BlockSize != 0 {
					return "", errors.New("plaintext is not a multiple of the block size")
				}

				ciphertext := make([]byte, len(b))
				stream := cipher.NewCBCEncrypter(block, iv)
				stream.CryptBlocks(ciphertext, b)
				return string(ciphertext), nil
			}
		default:
			return nil, fmt.Errorf("unrecognized encryption type: %v", schemeStr)
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var res string
			var err error
			switch t := v.(type) {
			case string:
				res, err = schemeFn([]byte(t))
			case []byte:
				res, err = schemeFn(t)
			default:
				err = value.NewTypeError(v, value.TString)
			}
			return res, err
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"decrypt_aes", "",
	).InCategory(
		MethodCategoryEncoding,
		"Decrypts an encrypted string or byte array target according to a chosen AES encryption method and returns the result as a byte array. The algorithms require a key and an initialization vector / nonce. Available schemes are: `ctr`, `gcm`, `ofb`, `cbc`.",
		NewExampleSpec("",
			`let key = "2b7e151628aed2a6abf7158809cf4f3c".decode("hex")
let vector = "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff".decode("hex")
root.decrypted = this.value.decode("hex").decrypt_aes("ctr", $key, $vector).string()`,
			`{"value":"84e9b31ff7400bdf80be7254"}`,
			`{"decrypted":"hello world!"}`,
		),
	).
		Param(ParamString("scheme", "The scheme to use for decryption, one of `ctr`, `gcm`, `ofb`, `cbc`.")).
		Param(ParamString("key", "A key to decrypt with.")).
		Param(ParamString("iv", "An initialization vector / nonce.")),
	func(args *ParsedParams) (simpleMethod, error) {
		schemeStr, err := args.FieldString("scheme")
		if err != nil {
			return nil, err
		}

		keyStr, err := args.FieldString("key")
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher([]byte(keyStr))
		if err != nil {
			return nil, err
		}

		ivStr, err := args.FieldString("iv")
		if err != nil {
			return nil, err
		}
		iv := []byte(ivStr)
		switch schemeStr {
		case "ctr":
			fallthrough
		case "ofb":
			fallthrough
		case "cbc":
			if len(iv) != block.BlockSize() {
				return nil, errors.New("the key must match the initialisation vector size")
			}
		}

		var schemeFn func([]byte) ([]byte, error)
		switch schemeStr {
		case "ctr":
			schemeFn = func(b []byte) ([]byte, error) {
				plaintext := make([]byte, len(b))
				stream := cipher.NewCTR(block, iv)
				stream.XORKeyStream(plaintext, b)
				return plaintext, nil
			}
		case "gcm":
			schemeFn = func(b []byte) ([]byte, error) {
				plaintext := make([]byte, 0, len(b))
				stream, err := cipher.NewGCM(block)
				if err != nil {
					return nil, fmt.Errorf("creating gcm failed: %w", err)
				}
				plaintext, err = stream.Open(plaintext, iv, b, nil)
				if err != nil {
					return nil, fmt.Errorf("gcm decrypting failed: %w", err)
				}
				return plaintext, nil
			}
		case "ofb":
			schemeFn = func(b []byte) ([]byte, error) {
				plaintext := make([]byte, len(b))
				//nolint:staticcheck // SA1019: cipher.NewOFB has been deprecated since Go 1.24
				stream := cipher.NewOFB(block, iv)
				stream.XORKeyStream(plaintext, b)
				return plaintext, nil
			}
		case "cbc":
			schemeFn = func(b []byte) ([]byte, error) {
				if len(b)%aes.BlockSize != 0 {
					return nil, errors.New("ciphertext is not a multiple of the block size")
				}
				stream := cipher.NewCBCDecrypter(block, iv)
				stream.CryptBlocks(b, b)
				return b, nil
			}
		default:
			return nil, fmt.Errorf("unrecognized decryption type: %v", schemeStr)
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var res []byte
			var err error
			switch t := v.(type) {
			case string:
				res, err = schemeFn([]byte(t))
			case []byte:
				res, err = schemeFn(t)
			default:
				err = value.NewTypeError(v, value.TString)
			}
			return res, err
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"escape_html", "",
	).InCategory(
		MethodCategoryStrings,
		"Escapes special HTML characters (`<`, `>`, `&`, `'`, `\"`) to make a string safe for HTML output. Use when embedding untrusted text in HTML to prevent XSS vulnerabilities.",
		NewExampleSpec("",
			`root.escaped = this.value.escape_html()`,
			`{"value":"foo & bar"}`,
			`{"escaped":"foo &amp; bar"}`,
		),
		NewExampleSpec("",
			`root.safe_html = this.user_input.escape_html()`,
			`{"user_input":"<script>alert('xss')</script>"}`,
			`{"safe_html":"&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return html.EscapeString(s), nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"index_of", "",
	).InCategory(
		MethodCategoryStrings,
		"Finds the position of a substring within a string. Returns the zero-based index of the first occurrence, or -1 if not found. Useful for searching and string manipulation.",
		NewExampleSpec("",
			`root.index = this.thing.index_of("bar")`,
			`{"thing":"foobar"}`,
			`{"index":3}`,
		),
		NewExampleSpec("",
			`root.index = content().index_of("meow")`,
			`the cat meowed, the dog woofed`,
			`{"index":8}`,
		),
	).Param(ParamString("value", "A string to search for.")),
	func(args *ParsedParams) (simpleMethod, error) {
		substring, err := args.FieldString("value")
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return int64(strings.Index(t, substring)), nil
			case []byte:
				return int64(bytes.Index(t, []byte(substring))), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"unescape_html", "",
	).InCategory(
		MethodCategoryStrings,
		"Converts HTML entities back to their original characters. Handles named entities (`&amp;`, `&lt;`), decimal (`&#225;`), and hexadecimal (`&xE1;`) formats. Use for processing HTML content or decoding HTML-escaped data.",
		NewExampleSpec("",
			`root.unescaped = this.value.unescape_html()`,
			`{"value":"foo &amp; bar"}`,
			`{"unescaped":"foo & bar"}`,
		),
		NewExampleSpec("",
			`root.text = this.html.unescape_html()`,
			`{"html":"&lt;p&gt;Hello &amp; goodbye&lt;/p&gt;"}`,
			`{"text":"<p>Hello & goodbye</p>"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return html.UnescapeString(s), nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"escape_url_query", "",
	).InCategory(
		MethodCategoryStrings,
		"Encodes a string for safe use in URL query parameters. Converts spaces to `+` and special characters to percent-encoded values. Use when building URLs with dynamic query parameters.",
		NewExampleSpec("",
			`root.escaped = this.value.escape_url_query()`,
			`{"value":"foo & bar"}`,
			`{"escaped":"foo+%26+bar"}`,
		),
		NewExampleSpec("",
			`root.url = "https://example.com?search=" + this.query.escape_url_query()`,
			`{"query":"hello world!"}`,
			`{"url":"https://example.com?search=hello+world%21"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return url.QueryEscape(s), nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"unescape_url_query", "",
	).InCategory(
		MethodCategoryStrings,
		"Decodes URL query parameter encoding, converting `+` to spaces and percent-encoded characters to their original values. Use for parsing URL query parameters.",
		NewExampleSpec("",
			`root.unescaped = this.value.unescape_url_query()`,
			`{"value":"foo+%26+bar"}`,
			`{"unescaped":"foo & bar"}`,
		),
		NewExampleSpec("",
			`root.search = this.param.unescape_url_query()`,
			`{"param":"hello+world%21"}`,
			`{"search":"hello world!"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return url.QueryUnescape(s)
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"filepath_join", "",
	).InCategory(
		MethodCategoryStrings,
		"Combines an array of path components into a single OS-specific file path using the correct separator (`/` on Unix, `\\` on Windows). Use for constructing file paths from components.",
		NewExampleSpec("",
			`root.path = this.path_elements.filepath_join()`,
			strings.ReplaceAll(`{"path_elements":["/foo/","bar.txt"]}`, "/", string(filepath.Separator)),
			strings.ReplaceAll(`{"path":"/foo/bar.txt"}`, "/", string(filepath.Separator)),
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			arr, ok := v.([]any)
			if !ok {
				return nil, value.NewTypeError(v, value.TArray)
			}
			strs := make([]string, 0, len(arr))
			for i, ele := range arr {
				str, err := value.IGetString(ele)
				if err != nil {
					return nil, fmt.Errorf("path element %v: %w", i, err)
				}
				strs = append(strs, str)
			}
			return filepath.Join(strs...), nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"filepath_split", "",
	).InCategory(
		MethodCategoryStrings,
		"Separates a file path into directory and filename components, returning a two-element array `[directory, filename]`. Use for extracting the filename or directory from a full path.",
		NewExampleSpec("",
			`root.path_sep = this.path.filepath_split()`,
			strings.ReplaceAll(`{"path":"/foo/bar.txt"}`, "/", string(filepath.Separator)),
			strings.ReplaceAll(`{"path_sep":["/foo/","bar.txt"]}`, "/", string(filepath.Separator)),
			`{"path":"baz.txt"}`,
			`{"path_sep":["","baz.txt"]}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			dir, file := filepath.Split(s)
			return []any{dir, file}, nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"format", "",
	).InCategory(
		MethodCategoryStrings,
		"Formats a string using Go's printf-style formatting with the string as the format template. Supports all Go format verbs (`%s`, `%d`, `%v`, etc.). Use for building formatted strings from dynamic values.",
		NewExampleSpec("",
			`root.foo = "%s(%v): %v".format(this.name, this.age, this.fingers)`,
			`{"name":"lance","age":37,"fingers":13}`,
			`{"foo":"lance(37): 13"}`,
		),
		NewExampleSpec("",
			`root.message = "User %s has %v points".format(this.username, this.score)`,
			`{"username":"alice","score":100}`,
			`{"message":"User alice has 100 points"}`,
		),
	).VariadicParams(),
	func(args *ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return fmt.Sprintf(s, args.Raw()...), nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"has_prefix", "",
	).InCategory(
		MethodCategoryStrings,
		"Tests if a string starts with a specified prefix. Returns `true` if the string begins with the prefix, `false` otherwise. Use for conditional logic based on string patterns.",
		NewExampleSpec("",
			`root.t1 = this.v1.has_prefix("foo")
root.t2 = this.v2.has_prefix("foo")`,
			`{"v1":"foobar","v2":"barfoo"}`,
			`{"t1":true,"t2":false}`,
		),
	).Param(ParamString("value", "The string to test.")),
	func(args *ParsedParams) (simpleMethod, error) {
		prefix, err := args.FieldString("value")
		if err != nil {
			return nil, err
		}
		prefixB := []byte(prefix)
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.HasPrefix(t, prefix), nil
			case []byte:
				return bytes.HasPrefix(t, prefixB), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"has_suffix", "",
	).InCategory(
		MethodCategoryStrings,
		"Tests if a string ends with a specified suffix. Returns `true` if the string ends with the suffix, `false` otherwise. Use for filtering or routing based on file extensions or string patterns.",
		NewExampleSpec("",
			`root.t1 = this.v1.has_suffix("foo")
root.t2 = this.v2.has_suffix("foo")`,
			`{"v1":"foobar","v2":"barfoo"}`,
			`{"t1":false,"t2":true}`,
		),
	).Param(ParamString("value", "The string to test.")),
	func(args *ParsedParams) (simpleMethod, error) {
		suffix, err := args.FieldString("value")
		if err != nil {
			return nil, err
		}
		suffixB := []byte(suffix)
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.HasSuffix(t, suffix), nil
			case []byte:
				return bytes.HasSuffix(t, suffixB), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"hash", "",
	).InCategory(
		MethodCategoryEncoding,
		`
Hashes a string or byte array according to a chosen algorithm and returns the result as a byte array. When mapping the result to a JSON field the value should be cast to a string using the method `+"xref:guides:bloblang/methods.adoc#string[`string`], or encoded using the method xref:guides:bloblang/methods.adoc#encode[`encode`]"+`, otherwise it will be base64 encoded by default.

Available algorithms are: `+"`hmac_sha1`, `hmac_sha256`, `hmac_sha512`, `md5`, `sha1`, `sha256`, `sha512`, `xxhash64`, `crc32`, `fnv32`"+`.

The following algorithms require a key, which is specified as a second argument: `+"`hmac_sha1`, `hmac_sha256`, `hmac_sha512`"+`.`,
		NewExampleSpec("",
			`root.h1 = this.value.hash("sha1").encode("hex")
root.h2 = this.value.hash("hmac_sha1","static-key").encode("hex")`,
			`{"value":"hello world"}`,
			`{"h1":"2aae6c35c94fcfb415dbe95f408b9ce91ee846ed","h2":"d87e5f068fa08fe90bb95bc7c8344cb809179d76"}`,
		),
		NewExampleSpec("The `crc32` algorithm supports options for the polynomial.",
			`root.h1 = this.value.hash(algorithm: "crc32", polynomial: "Castagnoli").encode("hex")
root.h2 = this.value.hash(algorithm: "crc32", polynomial: "Koopman").encode("hex")`,
			`{"value":"hello world"}`,
			`{"h1":"c99465aa","h2":"df373d3c"}`,
		),
	).
		Param(ParamString("algorithm", "The hasing algorithm to use.")).
		Param(ParamString("key", "An optional key to use.").Optional()).
		Param(ParamString("polynomial", "An optional polynomial key to use when selecting the `crc32` algorithm, otherwise ignored. Options are `IEEE` (default), `Castagnoli` and `Koopman`").Default("IEEE")),
	func(args *ParsedParams) (simpleMethod, error) {
		algorithmStr, err := args.FieldString("algorithm")
		if err != nil {
			return nil, err
		}
		var key []byte
		keyParam, err := args.FieldOptionalString("key")
		if err != nil {
			return nil, err
		}
		if keyParam != nil {
			key = []byte(*keyParam)
		}
		poly, err := args.FieldString("polynomial")
		if err != nil {
			return nil, err
		}
		var hashFn func([]byte) ([]byte, error)
		switch algorithmStr {
		case "hmac_sha1", "hmac-sha1":
			if len(key) == 0 {
				return nil, fmt.Errorf("hash algorithm %v requires a key argument", algorithmStr)
			}
			hashFn = func(b []byte) ([]byte, error) {
				hasher := hmac.New(sha1.New, key)
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "hmac_sha256", "hmac-sha256":
			if len(key) == 0 {
				return nil, fmt.Errorf("hash algorithm %v requires a key argument", algorithmStr)
			}
			hashFn = func(b []byte) ([]byte, error) {
				hasher := hmac.New(sha256.New, key)
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "hmac_sha512", "hmac-sha512":
			if len(key) == 0 {
				return nil, fmt.Errorf("hash algorithm %v requires a key argument", algorithmStr)
			}
			hashFn = func(b []byte) ([]byte, error) {
				hasher := hmac.New(sha512.New, key)
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "md5":
			hashFn = func(b []byte) ([]byte, error) {
				hasher := md5.New()
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "sha1":
			hashFn = func(b []byte) ([]byte, error) {
				hasher := sha1.New()
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "sha256":
			hashFn = func(b []byte) ([]byte, error) {
				hasher := sha256.New()
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "sha512":
			hashFn = func(b []byte) ([]byte, error) {
				hasher := sha512.New()
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "xxhash64":
			hashFn = func(b []byte) ([]byte, error) {
				h := xxhash.New64()
				_, _ = h.Write(b)
				return []byte(strconv.FormatUint(h.Sum64(), 10)), nil
			}
		case "crc32":
			hashFn = func(b []byte) ([]byte, error) {
				var hasher hash.Hash
				switch poly {
				case "IEEE":
					hasher = crc32.NewIEEE()
				case "Castagnoli":
					hasher = crc32.New(crc32.MakeTable(crc32.Castagnoli))
				case "Koopman":
					hasher = crc32.New(crc32.MakeTable(crc32.Koopman))
				default:
					return nil, fmt.Errorf("unsupported crc32 hash key %q", poly)
				}
				_, _ = hasher.Write(b)
				return hasher.Sum(nil), nil
			}
		case "fnv32":
			hashFn = func(b []byte) ([]byte, error) {
				h := fnv.New32()
				_, _ = h.Write(b)
				return []byte(strconv.FormatUint(uint64(h.Sum32()), 10)), nil
			}
		default:
			return nil, fmt.Errorf("unrecognized hash type: %v", algorithmStr)
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var res []byte
			var err error
			switch t := v.(type) {
			case string:
				res, err = hashFn([]byte(t))
			case []byte:
				res, err = hashFn(t)
			default:
				err = value.NewTypeError(v, value.TString)
			}
			return res, err
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"uuid_v5", "",
	).InCategory(
		MethodCategoryEncoding,
		`
Returns UUID version 5 for the given string.`,
		NewExampleSpec("", `root.id = "example".uuid_v5()`, `{"id": "feb54431-301b-52bb-a6dd-e1e93e81bb9e"}`),
		NewExampleSpec("", `root.id = "example".uuid_v5("x500")`, `{"id": "0cbd148f-768f-52fe-a1cd-0c4e6c65de91"}`),
		NewExampleSpec("", `root.id = "example".uuid_v5("77f836b7-9f61-46c0-851e-9b6ca3535e69")`, `{"id": "a0d220eb-18f1-50ca-b888-86aa5b604edf"}`),
	).Param(ParamString("ns", "An optional namespace name or UUID. It supports the `dns`, `url`, `oid` and `x500` predefined namespaces and any valid RFC-9562 UUID. If empty, the nil UUID will be used.").Optional()),
	func(args *ParsedParams) (simpleMethod, error) {
		ns, err := args.FieldOptionalString("ns")
		if err != nil {
			return nil, err
		}

		return func(v any, ctx FunctionContext) (any, error) {
			if v == nil {
				return nil, nil
			}

			var uns uuid.UUID
			if ns == nil {
				uns = uuid.Nil
			} else {
				switch *ns {
				case "dns", "DNS":
					uns = uuid.NamespaceDNS
				case "url", "URL":
					uns = uuid.NamespaceURL
				case "oid", "OID":
					uns = uuid.NamespaceOID
				case "x500", "X500":
					uns = uuid.NamespaceX500
				default:
					uns, err = uuid.FromString(*ns)
					if err != nil {
						return nil, fmt.Errorf("invalid ns uuid: %q", *ns)
					}
				}
			}

			switch t := v.(type) {
			case string:
				return uuid.NewV5(uns, t).String(), nil
			case []byte:
				return uuid.NewV5(uns, string(t)).String(), nil
			}

			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"join", "",
	).InCategory(
		MethodCategoryObjectAndArray,
		"Concatenates an array of strings into a single string with an optional delimiter between elements. Use for building CSV strings, URLs, or combining text fragments.",
		NewExampleSpec("",
			`root.joined_words = this.words.join()
root.joined_numbers = this.numbers.map_each(this.string()).join(",")`,
			`{"words":["hello","world"],"numbers":[3,8,11]}`,
			`{"joined_numbers":"3,8,11","joined_words":"helloworld"}`,
		),
	).Param(ParamString("delimiter", "An optional delimiter to add between each string.").Optional()),
	func(args *ParsedParams) (simpleMethod, error) {
		delimArg, err := args.FieldOptionalString("delimiter")
		if err != nil {
			return nil, err
		}
		delim := ""
		if delimArg != nil {
			delim = *delimArg
		}
		return func(v any, ctx FunctionContext) (any, error) {
			slice, ok := v.([]any)
			if !ok {
				return nil, value.NewTypeError(v, value.TArray)
			}

			var buf bytes.Buffer
			for i, sv := range slice {
				if i > 0 {
					_, _ = buf.WriteString(delim)
				}
				switch t := sv.(type) {
				case string:
					_, _ = buf.WriteString(t)
				case []byte:
					_, _ = buf.Write(t)
				default:
					return nil, fmt.Errorf("failed to join element %v: %w", i, value.NewTypeError(sv, value.TString))
				}
			}
			return buf.String(), nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"uppercase", "",
	).InCategory(
		MethodCategoryStrings,
		"Converts all letters in a string to uppercase. Use for case-insensitive comparisons or formatting output.",
		NewExampleSpec("",
			`root.foo = this.foo.uppercase()`,
			`{"foo":"hello world"}`,
			`{"foo":"HELLO WORLD"}`,
		),
		NewExampleSpec("",
			`root.code = this.product_code.uppercase()`,
			`{"product_code":"abc-123"}`,
			`{"code":"ABC-123"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.ToUpper(t), nil
			case []byte:
				return bytes.ToUpper(t), nil
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"lowercase", "",
	).InCategory(
		MethodCategoryStrings,
		"Converts all letters in a string to lowercase. Use for case-insensitive comparisons, normalization, or formatting output.",
		NewExampleSpec("",
			`root.foo = this.foo.lowercase()`,
			`{"foo":"HELLO WORLD"}`,
			`{"foo":"hello world"}`,
		),
		NewExampleSpec("",
			`root.email = this.user_email.lowercase()`,
			`{"user_email":"User@Example.COM"}`,
			`{"email":"user@example.com"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.ToLower(t), nil
			case []byte:
				return bytes.ToLower(t), nil
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"parse_csv", "",
	).InCategory(
		MethodCategoryParsing,
		"Attempts to parse a string into an array of objects by following the CSV format described in RFC 4180.",
		NewExampleSpec("Parses CSV data with a header row",
			`root.orders = this.orders.parse_csv()`,
			`{"orders":"foo,bar\nfoo 1,bar 1\nfoo 2,bar 2"}`,
			`{"orders":[{"bar":"bar 1","foo":"foo 1"},{"bar":"bar 2","foo":"foo 2"}]}`,
		),
		NewExampleSpec("Parses CSV data without a header row",
			`root.orders = this.orders.parse_csv(false)`,
			`{"orders":"foo 1,bar 1\nfoo 2,bar 2"}`,
			`{"orders":[["foo 1","bar 1"],["foo 2","bar 2"]]}`,
		),
		NewExampleSpec("Parses CSV data delimited by dots",
			`root.orders = this.orders.parse_csv(delimiter:".")`,
			`{"orders":"foo.bar\nfoo 1.bar 1\nfoo 2.bar 2"}`,
			`{"orders":[{"bar":"bar 1","foo":"foo 1"},{"bar":"bar 2","foo":"foo 2"}]}`,
		),
		NewExampleSpec("Parses CSV data containing a quote in an unquoted field",
			`root.orders = this.orders.parse_csv(lazy_quotes:true)`,
			`{"orders":"foo,bar\nfoo 1,bar 1\nfoo\" \"2,bar\" \"2"}`,
			`{"orders":[{"bar":"bar 1","foo":"foo 1"},{"bar":"bar\" \"2","foo":"foo\" \"2"}]}`,
		)).
		Param(ParamBool("parse_header_row", "Whether to reference the first row as a header row. If set to true the output structure for messages will be an object where field keys are determined by the header row. Otherwise, the output will be an array of row arrays.").Default(true)).
		Param(ParamString("delimiter", "The delimiter to use for splitting values in each record. It must be a single character.").Default(",")).
		Param(ParamBool("lazy_quotes", "If set to `true`, a quote may appear in an unquoted field and a non-doubled quote may appear in a quoted field.").Default(false)),
	parseCSVMethod,
)

func parseCSVMethod(args *ParsedParams) (simpleMethod, error) {
	return func(v any, ctx FunctionContext) (any, error) {
		var parseHeaderRow bool
		var optBool *bool
		var err error
		if optBool, err = args.FieldOptionalBool("parse_header_row"); err != nil {
			return nil, err
		}
		parseHeaderRow = *optBool

		var delimiter rune
		var optString *string
		if optString, err = args.FieldOptionalString("delimiter"); err != nil {
			return nil, err
		}
		delimRunes := []rune(*optString)
		if len(delimRunes) != 1 {
			return nil, errors.New("delimiter value must be exactly one character")
		}
		delimiter = delimRunes[0]

		var lazyQuotes bool
		if optBool, err = args.FieldOptionalBool("lazy_quotes"); err != nil {
			return nil, err
		}
		lazyQuotes = *optBool

		var csvBytes []byte
		switch t := v.(type) {
		case string:
			csvBytes = []byte(t)
		case []byte:
			csvBytes = t
		default:
			return nil, value.NewTypeError(v, value.TString)
		}

		r := csv.NewReader(bytes.NewReader(csvBytes))
		r.Comma = delimiter
		r.LazyQuotes = lazyQuotes
		strRecords, err := r.ReadAll()
		if err != nil {
			return nil, err
		}
		if len(strRecords) == 0 {
			return nil, errors.New("zero records were parsed")
		}

		var records []any
		if parseHeaderRow {
			records = make([]any, 0, len(strRecords)-1)
			headers := strRecords[0]
			if len(headers) == 0 {
				return nil, errors.New("no headers found on first row")
			}
			for j, strRecord := range strRecords[1:] {
				if len(headers) != len(strRecord) {
					return nil, fmt.Errorf("record on line %v: record mismatch with headers", j)
				}
				obj := make(map[string]any, len(strRecord))
				for i, r := range strRecord {
					obj[headers[i]] = r
				}
				records = append(records, obj)
			}
		} else {
			records = make([]any, 0, len(strRecords))
			for _, rec := range strRecords {
				records = append(records, toAnySlice(rec))
			}
		}

		return records, nil
	}, nil
}

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"parse_json", "",
	).Param(
		ParamBool("use_number", "An optional flag that when set makes parsing numbers as json.Number instead of the default float64.").Optional(),
	).InCategory(
		MethodCategoryParsing,
		"Attempts to parse a string as a JSON document and returns the result.",
		NewExampleSpec("",
			`root.doc = this.doc.parse_json()`,
			`{"doc":"{\"foo\":\"bar\"}"}`,
			`{"doc":{"foo":"bar"}}`,
		),
		NewExampleSpec("",
			`root.doc = this.doc.parse_json(use_number: true)`,
			`{"doc":"{\"foo\":\"11380878173205700000000000000000000000000000000\"}"}`,
			`{"doc":{"foo":"11380878173205700000000000000000000000000000000"}}`,
		),
	),
	func(args *ParsedParams) (simpleMethod, error) {
		useNumber, err := args.FieldOptionalBool("use_number")
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var jsonBytes []byte
			switch t := v.(type) {
			case string:
				jsonBytes = []byte(t)
			case []byte:
				jsonBytes = t
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			var jObj any
			decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
			if useNumber != nil && *useNumber {
				decoder.UseNumber()
			}
			if err := decoder.Decode(&jObj); err != nil {
				return nil, fmt.Errorf("failed to parse value as JSON: %w", err)
			}
			return jObj, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"parse_yaml", "",
	).InCategory(
		MethodCategoryParsing,
		"Attempts to parse a string as a single YAML document and returns the result.",
		NewExampleSpec("",
			`root.doc = this.doc.parse_yaml()`,
			`{"doc":"foo: bar"}`,
			`{"doc":{"foo":"bar"}}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			var yamlBytes []byte
			switch t := v.(type) {
			case string:
				yamlBytes = []byte(t)
			case []byte:
				yamlBytes = t
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			var sObj any
			if err := yaml.Unmarshal(yamlBytes, &sObj); err != nil {
				return nil, fmt.Errorf("failed to parse value as YAML: %w", err)
			}
			return sObj, nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"format_yaml", "",
	).InCategory(
		MethodCategoryParsing,
		"Serializes a target value into a YAML byte array.",
		NewExampleSpec("",
			`root = this.doc.format_yaml()`,
			`{"doc":{"foo":"bar"}}`,
			`foo: bar
`,
		),
		NewExampleSpec("Use the `.string()` method in order to coerce the result into a string.",
			`root.doc = this.doc.format_yaml().string()`,
			`{"doc":{"foo":"bar"}}`,
			`{"doc":"foo: bar\n"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			return yaml.Marshal(v)
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"format_json", "",
	).InCategory(
		MethodCategoryParsing,
		"Serializes a target value into a pretty-printed JSON byte array (with 4 space indentation by default).",
		NewExampleSpec("",
			`root = this.doc.format_json()`,
			`{"doc":{"foo":"bar"}}`,
			`{
    "foo": "bar"
}`,
		),
		NewExampleSpec("Pass a string to the `indent` parameter in order to customise the indentation.",
			`root = this.format_json("  ")`,
			`{"doc":{"foo":"bar"}}`,
			`{
  "doc": {
    "foo": "bar"
  }
}`,
		),
		NewExampleSpec("Use the `.string()` method in order to coerce the result into a string.",
			`root.doc = this.doc.format_json().string()`,
			`{"doc":{"foo":"bar"}}`,
			`{"doc":"{\n    \"foo\": \"bar\"\n}"}`,
		),
		NewExampleSpec("Set the `no_indent` parameter to true to disable indentation. The result is equivalent to calling `bytes()`.",
			`root = this.doc.format_json(no_indent: true)`,
			`{"doc":{"foo":"bar"}}`,
			`{"foo":"bar"}`,
		),
		NewExampleSpec("Escapes problematic HTML characters.",
			`root = this.doc.format_json()`,
			`{"doc":{"email":"foo&bar@benthos.dev","name":"foo>bar"}}`,
			`{
    "email": "foo\u0026bar@benthos.dev",
    "name": "foo\u003ebar"
}`,
		),
		NewExampleSpec("Set the `escape_html` parameter to false to disable escaping of problematic HTML characters.",
			`root = this.doc.format_json(escape_html: false)`,
			`{"doc":{"email":"foo&bar@benthos.dev","name":"foo>bar"}}`,
			`{
    "email": "foo&bar@benthos.dev",
    "name": "foo>bar"
}`,
		),
	).
		Beta().
		Param(ParamString(
			"indent",
			"Indentation string. Each element in a JSON object or array will begin on a new, indented line followed by one or more copies of indent according to the indentation nesting.",
		).Default(strings.Repeat(" ", 4))).
		Param(ParamBool(
			"no_indent",
			"Disable indentation.",
		).Default(false)).
		Param(ParamBool(
			"escape_html",
			"Escape problematic HTML characters.",
		).Default(true)),
	func(args *ParsedParams) (simpleMethod, error) {
		indentOpt, err := args.FieldOptionalString("indent")
		if err != nil {
			return nil, err
		}
		indent := ""
		if indentOpt != nil {
			indent = *indentOpt
		}
		noIndentOpt, err := args.FieldOptionalBool("no_indent")
		if err != nil {
			return nil, err
		}
		escapeHTMLOpt, err := args.FieldOptionalBool("escape_html")
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			buffer := &bytes.Buffer{}

			encoder := json.NewEncoder(buffer)
			if !*noIndentOpt {
				encoder.SetIndent("", indent)
			}
			if !*escapeHTMLOpt {
				encoder.SetEscapeHTML(false)
			}

			if err := encoder.Encode(v); err != nil {
				return nil, err
			}

			// This hack is here because `format_json()` initially relied on `json.Marshal()` or `json.MarshalIndent()`
			// which don't add a trailing newline to the output and, also, other `format_*` methods in bloblang don't
			// append a trailing newline.
			return bytes.TrimRight(buffer.Bytes(), "\n"), nil
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"parse_url", "Attempts to parse a URL from a string value, returning a structured result that describes the various facets of the URL. The fields returned within the structured result roughly follow https://pkg.go.dev/net/url#URL, and may be expanded in future in order to present more information.",
	).InCategory(
		MethodCategoryParsing, "",
		NewExampleSpec("",
			`root.foo_url = this.foo_url.parse_url()`,
			`{"foo_url":"https://docs.redpanda.com/redpanda-connect/guides/bloblang/about/"}`,
			`{"foo_url":{"fragment":"","host":"docs.redpanda.com","opaque":"","path":"/redpanda-connect/guides/bloblang/about/","raw_fragment":"","raw_path":"","raw_query":"","scheme":"https"}}`,
		),
		NewExampleSpec("",
			`root.username = this.url.parse_url().user.name | "unknown"`,
			`{"url":"amqp://foo:bar@127.0.0.1:5672/"}`,
			`{"username":"foo"}`,
			`{"url":"redis://localhost:6379"}`,
			`{"username":"unknown"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(data string) (any, error) {
			urlParsed, err := url.Parse(data)
			if err != nil {
				return nil, err
			}
			values := map[string]any{
				"scheme":       urlParsed.Scheme,
				"opaque":       urlParsed.Opaque,
				"host":         urlParsed.Host,
				"path":         urlParsed.Path,
				"raw_path":     urlParsed.RawPath,
				"raw_query":    urlParsed.RawQuery,
				"fragment":     urlParsed.Fragment,
				"raw_fragment": urlParsed.RawFragment,
			}
			if urlParsed.User != nil {
				userObj := map[string]any{
					"name": urlParsed.User.Username(),
				}
				if pass, exists := urlParsed.User.Password(); exists {
					userObj["password"] = pass
				}
				values["user"] = userObj
			}
			return values, nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"reverse", "",
	).InCategory(
		MethodCategoryStrings,
		"Reverses the order of characters in a string. Unicode-aware for proper handling of multi-byte characters. Use for creating palindrome checks or reversing text data.",
		NewExampleSpec("",
			`root.reversed = this.thing.reverse()`,
			`{"thing":"backwards"}`,
			`{"reversed":"sdrawkcab"}`,
		),
		NewExampleSpec("",
			`root = content().reverse()`,
			`{"thing":"backwards"}`,
			`}"sdrawkcab":"gniht"{`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				runes := []rune(t)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return string(runes), nil
			case []byte:
				result := make([]byte, len(t))
				for i, b := range t {
					result[len(t)-i-1] = b
				}
				return result, nil
			}

			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"quote", "",
	).InCategory(
		MethodCategoryStrings,
		"Wraps a string in double quotes and escapes special characters (newlines, tabs, etc.) using Go escape sequences. Use for generating string literals or preparing strings for JSON-like formats.",
		NewExampleSpec("",
			`root.quoted = this.thing.quote()`,
			`{"thing":"foo\nbar"}`,
			`{"quoted":"\"foo\\nbar\""}`,
		),
		NewExampleSpec("",
			`root.literal = this.text.quote()`,
			`{"text":"hello\tworld"}`,
			`{"literal":"\"hello\\tworld\""}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return strconv.Quote(s), nil
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"unquote", "",
	).InCategory(
		MethodCategoryStrings,
		"Removes surrounding quotes and interprets escape sequences (`\\n`, `\\t`, etc.) to their literal characters. Use for parsing quoted string literals.",
		NewExampleSpec("",
			`root.unquoted = this.thing.unquote()`,
			`{"thing":"\"foo\\nbar\""}`,
			`{"unquoted":"foo\nbar"}`,
		),
		NewExampleSpec("",
			`root.text = this.literal.unquote()`,
			`{"literal":"\"hello\\tworld\""}`,
			`{"text":"hello\tworld"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return stringMethod(func(s string) (any, error) {
			return strconv.Unquote(s)
		}), nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec("replace", "Replaces all occurrences of a substring with another string. Use for text transformation, cleaning data, or normalizing strings.").InCategory(
		MethodCategoryStrings, "",
	).
		Param(ParamString("old", "A string to match against.")).
		Param(ParamString("new", "A string to replace with.")),
	replaceAllImpl,
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"replace_all", "",
	).InCategory(
		MethodCategoryStrings,
		"Replaces all occurrences of a substring with another string. Use for text transformation, cleaning data, or normalizing strings.",
		NewExampleSpec("",
			`root.new_value = this.value.replace_all("foo","dog")`,
			`{"value":"The foo ate my homework"}`,
			`{"new_value":"The dog ate my homework"}`,
		),
		NewExampleSpec("",
			`root.clean = this.text.replace_all("  ", " ")`,
			`{"text":"hello  world  foo"}`,
			`{"clean":"hello world foo"}`,
		),
	).
		Param(ParamString("old", "A string to match against.")).
		Param(ParamString("new", "A string to replace with.")),
	replaceAllImpl,
)

func replaceAllImpl(args *ParsedParams) (simpleMethod, error) {
	oldStr, err := args.FieldString("old")
	if err != nil {
		return nil, err
	}
	newStr, err := args.FieldString("new")
	if err != nil {
		return nil, err
	}
	oldB, newB := []byte(oldStr), []byte(newStr)
	return func(v any, ctx FunctionContext) (any, error) {
		switch t := v.(type) {
		case string:
			return strings.ReplaceAll(t, oldStr, newStr), nil
		case []byte:
			return bytes.ReplaceAll(t, oldB, newB), nil
		}
		return nil, value.NewTypeError(v, value.TString)
	}, nil
}

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec("replace_many", "Performs multiple find-and-replace operations in sequence using an array of `[old, new]` pairs. More efficient than chaining multiple `replace_all` calls. Use for bulk text transformations.").InCategory(
		MethodCategoryStrings, "",
	).
		Param(ParamArray("values", "An array of values, each even value will be replaced with the following odd value.")),
	replaceAllManyImpl,
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"replace_all_many", "",
	).InCategory(
		MethodCategoryStrings,
		"Performs multiple find-and-replace operations in sequence using an array of `[old, new]` pairs. More efficient than chaining multiple `replace_all` calls. Use for bulk text transformations.",
		NewExampleSpec("",
			`root.new_value = this.value.replace_all_many([
  "<b>", "&lt;b&gt;",
  "</b>", "&lt;/b&gt;",
  "<i>", "&lt;i&gt;",
  "</i>", "&lt;/i&gt;",
])`,
			`{"value":"<i>Hello</i> <b>World</b>"}`,
			`{"new_value":"&lt;i&gt;Hello&lt;/i&gt; &lt;b&gt;World&lt;/b&gt;"}`,
		),
	).Param(ParamArray("values", "An array of values, each even value will be replaced with the following odd value.")),
	replaceAllManyImpl,
)

func replaceAllManyImpl(args *ParsedParams) (simpleMethod, error) {
	items, err := args.FieldArray("values")
	if err != nil {
		return nil, err
	}
	if len(items)%2 != 0 {
		return nil, fmt.Errorf("invalid arg, replacements should be in pairs and must therefore be even: %v", items)
	}

	var replacePairs [][2]string
	var replacePairsBytes [][2][]byte

	for i := 0; i < len(items); i += 2 {
		from, err := value.IGetString(items[i])
		if err != nil {
			return nil, fmt.Errorf("invalid replacement value at index %v: %w", i, err)
		}
		to, err := value.IGetString(items[i+1])
		if err != nil {
			return nil, fmt.Errorf("invalid replacement value at index %v: %w", i+1, err)
		}
		replacePairs = append(replacePairs, [2]string{from, to})
		replacePairsBytes = append(replacePairsBytes, [2][]byte{[]byte(from), []byte(to)})
	}

	return func(v any, ctx FunctionContext) (any, error) {
		switch t := v.(type) {
		case string:
			for _, pair := range replacePairs {
				t = strings.ReplaceAll(t, pair[0], pair[1])
			}
			return t, nil
		case []byte:
			for _, pair := range replacePairsBytes {
				t = bytes.ReplaceAll(t, pair[0], pair[1])
			}
			return t, nil
		}
		return nil, value.NewTypeError(v, value.TString)
	}, nil
}

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_find_all", "",
	).InCategory(
		MethodCategoryRegexp,
		"Finds all matches of a regular expression in a string and returns them as an array. Use for extracting multiple patterns or validating repeating structures.",
		NewExampleSpec("",
			`root.matches = this.value.re_find_all("a.")`,
			`{"value":"paranormal"}`,
			`{"matches":["ar","an","al"]}`,
		),
		NewExampleSpec("",
			`root.numbers = this.text.re_find_all("[0-9]+")`,
			`{"text":"I have 2 apples and 15 oranges"}`,
			`{"numbers":["2","15"]}`,
		),
	).Param(ParamString("pattern", "The pattern to match against.")),
	func(args *ParsedParams) (simpleMethod, error) {
		reStr, err := args.FieldString("pattern")
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var result []any
			switch t := v.(type) {
			case string:
				matches := re.FindAllString(t, -1)
				result = toAnySlice(matches)
			case []byte:
				matches := re.FindAll(t, -1)
				result = make([]any, 0, len(matches))
				for _, str := range matches {
					result = append(result, string(str))
				}
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			return result, nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_find_all_submatch", "",
	).InCategory(
		MethodCategoryRegexp,
		"Finds all regex matches and their capture groups, returning an array of arrays where each inner array contains the full match and captured subgroups. Use for extracting structured data with capture groups.",
		NewExampleSpec("",
			`root.matches = this.value.re_find_all_submatch("a(x*)b")`,
			`{"value":"-axxb-ab-"}`,
			`{"matches":[["axxb","xx"],["ab",""]]}`,
		),
		NewExampleSpec("",
			`root.emails = this.text.re_find_all_submatch("(\\w+)@(\\w+\\.\\w+)")`,
			`{"text":"Contact: alice@example.com or bob@test.org"}`,
			`{"emails":[["alice@example.com","alice","example.com"],["bob@test.org","bob","test.org"]]}`,
		),
	).Param(ParamString("pattern", "The pattern to match against.")),
	func(args *ParsedParams) (simpleMethod, error) {
		reStr, err := args.FieldString("pattern")
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var result []any
			switch t := v.(type) {
			case string:
				groupMatches := re.FindAllStringSubmatch(t, -1)
				result = make([]any, 0, len(groupMatches))
				for _, matches := range groupMatches {
					r := toAnySlice(matches)
					result = append(result, r)
				}
			case []byte:
				groupMatches := re.FindAllSubmatch(t, -1)
				result = make([]any, 0, len(groupMatches))
				for _, matches := range groupMatches {
					r := make([]any, 0, len(matches))
					for _, str := range matches {
						r = append(r, string(str))
					}
					result = append(result, r)
				}
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			return result, nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_find_object", "",
	).InCategory(
		MethodCategoryRegexp,
		"Finds the first regex match and returns an object with named capture groups as keys (or numeric indices for unnamed groups). The key \"0\" contains the full match. Use for parsing structured text into fields.",
		NewExampleSpec("",
			`root.matches = this.value.re_find_object("a(?P<foo>x*)b")`,
			`{"value":"-axxb-ab-"}`,
			`{"matches":{"0":"axxb","foo":"xx"}}`,
		),
		NewExampleSpec("",
			`root.matches = this.value.re_find_object("(?P<key>\\w+):\\s+(?P<value>\\w+)")`,
			`{"value":"option1: value1"}`,
			`{"matches":{"0":"option1: value1","key":"option1","value":"value1"}}`,
		),
	).Param(ParamString("pattern", "The pattern to match against.")),
	func(args *ParsedParams) (simpleMethod, error) {
		reStr, err := args.FieldString("pattern")
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, err
		}
		groups := re.SubexpNames()
		for i, k := range groups {
			if k == "" {
				groups[i] = strconv.Itoa(i)
			}
		}
		return func(v any, ctx FunctionContext) (any, error) {
			result := make(map[string]any, len(groups))
			switch t := v.(type) {
			case string:
				groupMatches := re.FindStringSubmatch(t)
				for i, match := range groupMatches {
					key := groups[i]
					result[key] = match
				}
			case []byte:
				groupMatches := re.FindSubmatch(t)
				for i, match := range groupMatches {
					key := groups[i]
					result[key] = match
				}
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			return result, nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_find_all_object", "",
	).InCategory(
		MethodCategoryRegexp,
		"Finds all regex matches and returns an array of objects with named capture groups as keys. Each object represents one match with its captured groups. Use for parsing multiple structured records from text.",
		NewExampleSpec("",
			`root.matches = this.value.re_find_all_object("a(?P<foo>x*)b")`,
			`{"value":"-axxb-ab-"}`,
			`{"matches":[{"0":"axxb","foo":"xx"},{"0":"ab","foo":""}]}`,
		),
		NewExampleSpec("",
			`root.matches = this.value.re_find_all_object("(?m)(?P<key>\\w+):\\s+(?P<value>\\w+)$")`,
			`{"value":"option1: value1\noption2: value2\noption3: value3"}`,
			`{"matches":[{"0":"option1: value1","key":"option1","value":"value1"},{"0":"option2: value2","key":"option2","value":"value2"},{"0":"option3: value3","key":"option3","value":"value3"}]}`,
		),
	).Param(ParamString("pattern", "The pattern to match against.")),
	func(args *ParsedParams) (simpleMethod, error) {
		reStr, err := args.FieldString("pattern")
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, err
		}
		groups := re.SubexpNames()
		for i, k := range groups {
			if k == "" {
				groups[i] = strconv.Itoa(i)
			}
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var result []any
			switch t := v.(type) {
			case string:
				reMatches := re.FindAllStringSubmatch(t, -1)
				result = make([]any, 0, len(reMatches))
				for _, matches := range reMatches {
					obj := make(map[string]any, len(groups))
					for i, match := range matches {
						key := groups[i]
						obj[key] = match
					}
					result = append(result, obj)
				}
			case []byte:
				reMatches := re.FindAllSubmatch(t, -1)
				result = make([]any, 0, len(reMatches))
				for _, matches := range reMatches {
					obj := make(map[string]any, len(groups))
					for i, match := range matches {
						key := groups[i]
						obj[key] = match
					}
					result = append(result, obj)
				}
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			return result, nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_match", "",
	).InCategory(
		MethodCategoryRegexp,
		"Tests if a regular expression matches anywhere in a string, returning `true` or `false`. Use for validation or conditional routing based on patterns.",
		NewExampleSpec("",
			`root.matches = this.value.re_match("[0-9]")`,
			`{"value":"there are 10 puppies"}`,
			`{"matches":true}`,
			`{"value":"there are ten puppies"}`,
			`{"matches":false}`,
		),
	).Param(ParamString("pattern", "The pattern to match against.")),
	func(args *ParsedParams) (simpleMethod, error) {
		reStr, err := args.FieldString("pattern")
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			var result bool
			switch t := v.(type) {
			case string:
				result = re.MatchString(t)
			case []byte:
				result = re.Match(t)
			default:
				return nil, value.NewTypeError(v, value.TString)
			}
			return result, nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec("re_replace", "Replaces all regex matches with a replacement string that can reference capture groups using `$1`, `$2`, etc. Use for pattern-based transformations or data reformatting.").InCategory(
		MethodCategoryRegexp, "",
	).
		Param(ParamString("pattern", "The pattern to match against.")).
		Param(ParamString("value", "The value to replace with.")),
	reReplaceAllImpl,
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"re_replace_all", "",
	).InCategory(
		MethodCategoryRegexp,
		"Replaces all regex matches with a replacement string that can reference capture groups using `$1`, `$2`, etc. Use for pattern-based transformations or data reformatting.",
		NewExampleSpec("",
			`root.new_value = this.value.re_replace_all("ADD ([0-9]+)","+($1)")`,
			`{"value":"foo ADD 70"}`,
			`{"new_value":"foo +(70)"}`,
		),
		NewExampleSpec("",
			`root.masked = this.email.re_replace_all("(\\w{2})\\w+@", "$1***@")`,
			`{"email":"alice@example.com"}`,
			`{"masked":"al***@example.com"}`,
		),
	).
		Param(ParamString("pattern", "The pattern to match against.")).
		Param(ParamString("value", "The value to replace with.")),
	reReplaceAllImpl,
)

func reReplaceAllImpl(args *ParsedParams) (simpleMethod, error) {
	reStr, err := args.FieldString("pattern")
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(reStr)
	if err != nil {
		return nil, err
	}
	with, err := args.FieldString("value")
	if err != nil {
		return nil, err
	}
	withBytes := []byte(with)
	return func(v any, ctx FunctionContext) (any, error) {
		var result string
		switch t := v.(type) {
		case string:
			result = re.ReplaceAllString(t, with)
		case []byte:
			result = string(re.ReplaceAll(t, withBytes))
		default:
			return nil, value.NewTypeError(v, value.TString)
		}
		return result, nil
	}, nil
}

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"split", "",
	).InCategory(
		MethodCategoryStrings,
		"Splits a string into an array of substrings using a delimiter. Use for parsing CSV-like data, splitting paths, or breaking text into tokens.",
		NewExampleSpec("",
			`root.new_value = this.value.split(",")`,
			`{"value":"foo,bar,baz"}`,
			`{"new_value":["foo","bar","baz"]}`,
		),
		NewExampleSpec("",
			`root.new_value = this.value.split(",", true)`,
			`{"value":"foo,,qux"}`,
			`{"new_value":["foo",null,"qux"]}`,
		),
		NewExampleSpec("",
			`root.words = this.sentence.split(" ")`,
			`{"sentence":"hello world from bloblang"}`,
			`{"words":["hello","world","from","bloblang"]}`,
		),
	).Param(ParamString("delimiter", "The delimiter to split with.")).
		Param(ParamBool("empty_as_null", "To treat empty substrings as null values").Default(false)),
	func(args *ParsedParams) (simpleMethod, error) {
		delim, err := args.FieldString("delimiter")
		if err != nil {
			return nil, err
		}
		emptyAsNull, err := args.FieldBool("empty_as_null")
		if err != nil {
			return nil, err
		}
		delimB := []byte(delim)
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				vals := strSplit(t, delim)
				if emptyAsNull {
					for i, v := range vals {
						if v == "" {
							vals[i] = nil
						}
					}
				}
				return vals, nil
			case []byte:
				vals := byteSplit(t, delimB)
				if emptyAsNull {
					for i, v := range vals {
						if len(v.([]byte)) == 0 {
							vals[i] = nil
						}
					}
				}
				return vals, nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

func toAnySlice[T any](slice []T) []any {
	out := make([]any, len(slice))
	for i, v := range slice {
		out[i] = v
	}
	return out
}

func strSplit(s string, sep string) []any {
	if len(sep) == 0 {
		return toAnySlice(strings.Split(s, sep))
	}
	n := min(strings.Count(s, sep)+1, len(s)+1)
	a := make([]any, n)
	n--
	i := 0
	for i < n {
		m := strings.Index(s, sep)
		if m < 0 {
			break
		}
		a[i] = s[:m]
		s = s[m+len(sep):]
		i++
	}
	a[i] = s
	return a[:i+1]
}

func byteSplit(s []byte, sep []byte) []any {
	if len(sep) == 0 {
		return toAnySlice(bytes.Split(s, sep))
	}
	n := min(bytes.Count(s, sep)+1, len(s)+1)
	a := make([]any, n)
	n--
	i := 0
	for i < n {
		m := bytes.Index(s, sep)
		if m < 0 {
			break
		}
		a[i] = s[:m]
		s = s[m+len(sep):]
		i++
	}
	a[i] = s
	return a[:i+1]
}

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"string", "",
	).InCategory(
		MethodCategoryCoercion,
		"Converts any value to its string representation. Numbers, booleans, and objects are converted to strings; existing strings are unchanged. Use for type coercion or creating string representations.",
		NewExampleSpec("",
			`root.nested_json = this.string()`,
			`{"foo":"bar"}`,
			`{"nested_json":"{\"foo\":\"bar\"}"}`,
		),
		NewExampleSpec("",
			`root.id = this.id.string()`,
			`{"id":228930314431312345}`,
			`{"id":"228930314431312345"}`,
		),
	),
	func(*ParsedParams) (simpleMethod, error) {
		return func(v any, ctx FunctionContext) (any, error) {
			return value.IToString(v), nil
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"trim", "",
	).InCategory(
		MethodCategoryStrings,
		"Removes leading and trailing characters from a string. Without arguments, removes whitespace. With a cutset argument, removes any characters in the cutset. Use for cleaning user input or normalizing strings.",
		NewExampleSpec("",
			`root.title = this.title.trim("!?")
root.description = this.description.trim()`,
			`{"description":"  something happened and its amazing! ","title":"!!!watch out!?"}`,
			`{"description":"something happened and its amazing!","title":"watch out"}`,
		),
	).Param(ParamString("cutset", "An optional string of characters to trim from the target value.").Optional()),
	func(args *ParsedParams) (simpleMethod, error) {
		cutset, err := args.FieldOptionalString("cutset")
		if err != nil {
			return nil, err
		}
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				if cutset == nil {
					return strings.TrimSpace(t), nil
				}
				return strings.Trim(t, *cutset), nil
			case []byte:
				if cutset == nil {
					return bytes.TrimSpace(t), nil
				}
				return bytes.Trim(t, *cutset), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"trim_prefix", "",
	).InCategory(
		MethodCategoryStrings,
		"Removes a specified prefix from the beginning of a string if present. If the string doesn't start with the prefix, returns the string unchanged. Use for stripping known prefixes from identifiers or paths.",
		NewExampleSpec("",
			`root.name = this.name.trim_prefix("foobar_")
root.description = this.description.trim_prefix("foobar_")`,
			`{"description":"unchanged","name":"foobar_blobton"}`,
			`{"description":"unchanged","name":"blobton"}`,
		),
	).Param(ParamString("prefix", "The leading prefix substring to trim from the string.")).
		AtVersion("4.12.0"),
	func(args *ParsedParams) (simpleMethod, error) {
		prefix, err := args.FieldString("prefix")
		if err != nil {
			return nil, err
		}
		bytesPrefix := []byte(prefix)
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.TrimPrefix(t, prefix), nil
			case []byte:
				return bytes.TrimPrefix(t, bytesPrefix), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

var _ = registerSimpleMethod(
	NewMethodSpec(
		"trim_suffix", "",
	).InCategory(
		MethodCategoryStrings,
		"Removes a specified suffix from the end of a string if present. If the string doesn't end with the suffix, returns the string unchanged. Use for stripping file extensions or known suffixes.",
		NewExampleSpec("",
			`root.name = this.name.trim_suffix("_foobar")
root.description = this.description.trim_suffix("_foobar")`,
			`{"description":"unchanged","name":"blobton_foobar"}`,
			`{"description":"unchanged","name":"blobton"}`,
		),
	).Param(ParamString("suffix", "The trailing suffix substring to trim from the string.")).
		AtVersion("4.12.0"),
	func(args *ParsedParams) (simpleMethod, error) {
		suffix, err := args.FieldString("suffix")
		if err != nil {
			return nil, err
		}
		bytesSuffix := []byte(suffix)
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				return strings.TrimSuffix(t, suffix), nil
			case []byte:
				return bytes.TrimSuffix(t, bytesSuffix), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)

//------------------------------------------------------------------------------

var _ = registerSimpleMethod(
	NewMethodSpec(
		"repeat", "",
	).InCategory(
		MethodCategoryStrings,
		"Creates a new string by repeating the input string a specified number of times. Use for generating padding, separators, or test data.",
		NewExampleSpec("",
			`root.repeated = this.name.repeat(3)
root.not_repeated = this.name.repeat(0)`,
			`{"name":"bob"}`,
			`{"not_repeated":"","repeated":"bobbobbob"}`,
		),
		NewExampleSpec("",
			`root.separator = "-".repeat(10)`,
			`{}`,
			`{"separator":"----------"}`,
		),
	).Param(ParamInt64("count", "The number of times to repeat the string.")),
	func(args *ParsedParams) (simpleMethod, error) {
		count, err := args.FieldInt64("count")
		if err != nil {
			return nil, err
		}
		if count < 0 {
			return nil, fmt.Errorf("invalid count, must be greater than or equal to zero: %d", count)
		}
		return func(v any, ctx FunctionContext) (any, error) {
			switch t := v.(type) {
			case string:
				hi, lo := bits.Mul(uint(len(t)), uint(count))
				if hi > 0 || lo > uint(math.MaxInt) {
					return nil, fmt.Errorf("invalid count, would overflow: %d*%d", len(t), count)
				}
				return strings.Repeat(t, int(count)), nil
			case []byte:
				hi, lo := bits.Mul(uint(len(t)), uint(count))
				if hi > 0 || lo > uint(math.MaxInt) {
					return nil, fmt.Errorf("invalid count, would overflow: %d*%d", len(t), count)
				}
				return bytes.Repeat(t, int(count)), nil
			}
			return nil, value.NewTypeError(v, value.TString)
		}, nil
	},
)
