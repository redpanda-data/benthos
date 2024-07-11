package log

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFromParsedWithWithStaticFieldStrings(t *testing.T) {

	spec := Spec()
	pConf, err := spec.ParsedConfigFromAny(map[string]any{
		"level":          "INFO",
		"format":         "logfmt",
		"add_timestamp":  false,
		"level_name":     "level",
		"timestamp_name": "time",
		"message_name":   "msg",
		"static_fields": map[string]any{
			"@service": "benthos",
		},
		"file": map[string]any{
			"path":                "",
			"rotate":              false,
			"rotate_max_age_days": 0,
		},
	})
	if err != nil {
		panic(err)
	}

	config, err := FromParsed(pConf)
	if err != nil {
		t.Fatal("Failure in FromParsed:", err)
	}
	expected := Config{
		LogLevel:      "INFO",
		Format:        "logfmt",
		LevelName:     "level",
		MessageName:   "msg",
		TimestampName: "time",
		StaticFields: map[string]any{
			"@service": "benthos",
		},
	}
	diff := cmp.Diff(config, expected)
	if diff != "" {
		t.Fatalf("Unexpected parsed config (-got +want):\n%s\n", diff)
	}
}

func TestFromParsedWithStaticFieldObjects(t *testing.T) {

	spec := Spec()
	pConf, err := spec.ParsedConfigFromAny(map[string]any{
		"level":         "INFO",
		"format":        "logfmt",
		"add_timestamp": false,
		"static_fields": map[string]any{
			"serviceContext": map[string]any{
				"service": "benthos",
			},
		},
	})
	if err != nil {
		panic(err)
	}

	config, err := FromParsed(pConf)
	if err != nil {
		t.Fatal("failure in FromParsed:", err)
	}
	expected := Config{
		LogLevel:      "INFO",
		Format:        "logfmt",
		LevelName:     "level",
		MessageName:   "msg",
		TimestampName: "time",
		StaticFields: map[string]any{
			"serviceContext": map[string]any{
				"service": string("benthos"),
			},
		},
	}
	diff := cmp.Diff(config, expected)
	if diff != "" {
		t.Fatalf("Unexpected parsed config (-got +want):\n%s\n", diff)
	}
}
