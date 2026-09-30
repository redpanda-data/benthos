// Copyright 2026 Redpanda Data, Inc.

package pure

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	jsonschema "github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/redpanda-data/benthos/v4/public/bloblangv2"
)

// V2 ports of V1 parsing methods that operate purely on the receiver value.

func init() {
	bloblangv2.MustRegisterMethod("parse_yaml",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Attempts to parse the receiver string as a single YAML document and returns the result."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return parseYAMLV2, nil
		},
	)

	bloblangv2.MustRegisterMethod("format_yaml",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Serialises the receiver value into a YAML byte array."),
		func(_ *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
			return func(v any) (any, error) {
				return yaml.Marshal(v)
			}, nil
		},
	)

	bloblangv2.MustRegisterMethod("parse_csv",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Attempts to parse the receiver string as RFC 4180 CSV. With a header row the result is an array of objects keyed by column; without one it is an array of row arrays.").
			Param(bloblangv2.NewBoolParam("parse_header_row").Description("Treat the first row as a header. When true the result is an array of objects keyed by column.").Default(true)).
			Param(bloblangv2.NewStringParam("delimiter").Description("Single-character field delimiter.").Default(",")).
			Param(bloblangv2.NewBoolParam("lazy_quotes").Description(`If true, allow a quote inside an unquoted field and a non-doubled quote in a quoted field.`).Default(false)),
		parseCSVV2Ctor,
	)

	bloblangv2.MustRegisterMethod("json_schema",
		bloblangv2.NewPluginSpec().
			Category("Parsing").
			Description("Validates the receiver value against a JSON schema and returns it unchanged on success, or an error describing the validation failure.").
			Param(bloblangv2.NewStringParam("schema").Description("A JSON schema document.")),
		jsonSchemaV2Ctor,
	)
}

func parseYAMLV2(v any) (any, error) {
	var data []byte
	switch t := v.(type) {
	case string:
		data = []byte(t)
	case []byte:
		data = t
	default:
		return nil, fmt.Errorf("expected string or bytes receiver, got %T", v)
	}
	var out any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse value as YAML: %w", err)
	}
	return normaliseYAMLNumbers(out), nil
}

// normaliseYAMLNumbers walks a yaml.Unmarshal result and rewrites any Go-int
// values to int64 to match the V2 numeric type discipline. yaml.v3 picks
// platform-sized int when it could fit; the V2 interpreter only knows int64
// / float64 / etc.
func normaliseYAMLNumbers(v any) any {
	switch t := v.(type) {
	case int:
		return int64(t)
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseYAMLNumbers(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseYAMLNumbers(vv)
		}
		return t
	default:
		return v
	}
}

func parseCSVV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	parseHeaderRow, err := args.GetBool("parse_header_row")
	if err != nil {
		return nil, err
	}
	delimStr, err := args.GetString("delimiter")
	if err != nil {
		return nil, err
	}
	delimRunes := []rune(delimStr)
	if len(delimRunes) != 1 {
		return nil, errors.New("delimiter value must be exactly one character")
	}
	delimiter := delimRunes[0]
	lazyQuotes, err := args.GetBool("lazy_quotes")
	if err != nil {
		return nil, err
	}

	return func(v any) (any, error) {
		var data []byte
		switch t := v.(type) {
		case string:
			data = []byte(t)
		case []byte:
			data = t
		default:
			return nil, fmt.Errorf("expected string or bytes receiver, got %T", v)
		}
		r := csv.NewReader(bytes.NewReader(data))
		r.Comma = delimiter
		r.LazyQuotes = lazyQuotes
		records, err := r.ReadAll()
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			return nil, errors.New("zero records were parsed")
		}
		if parseHeaderRow {
			headers := records[0]
			if len(headers) == 0 {
				return nil, errors.New("no headers found on first row")
			}
			out := make([]any, 0, len(records)-1)
			for j, rec := range records[1:] {
				if len(headers) != len(rec) {
					return nil, fmt.Errorf("record on line %d: record mismatch with headers", j)
				}
				obj := make(map[string]any, len(rec))
				for i, cell := range rec {
					obj[headers[i]] = cell
				}
				out = append(out, obj)
			}
			return out, nil
		}
		out := make([]any, 0, len(records))
		for _, rec := range records {
			row := make([]any, len(rec))
			for i, cell := range rec {
				row[i] = cell
			}
			out = append(out, row)
		}
		return out, nil
	}, nil
}

func jsonSchemaV2Ctor(args *bloblangv2.ParsedParams) (bloblangv2.Method, error) {
	schemaStr, err := args.GetString("schema")
	if err != nil {
		return nil, err
	}
	schema, err := jsonschema.NewSchema(jsonschema.NewStringLoader(schemaStr))
	if err != nil {
		return nil, fmt.Errorf("failed to parse json schema definition: %w", err)
	}
	return func(v any) (any, error) {
		result, err := schema.Validate(jsonschema.NewGoLoader(v))
		if err != nil {
			return nil, err
		}
		if result.Valid() {
			return v, nil
		}
		var b strings.Builder
		for i, desc := range result.Errors() {
			if i > 0 {
				b.WriteByte('\n')
			}
			description := strings.ToLower(desc.Description())
			if property := desc.Details()["property"]; property != nil {
				description = property.(string) + strings.TrimPrefix(description, strings.ToLower(property.(string)))
			}
			b.WriteString(desc.Field())
			b.WriteByte(' ')
			b.WriteString(description)
		}
		return nil, errors.New(b.String())
	}, nil
}
