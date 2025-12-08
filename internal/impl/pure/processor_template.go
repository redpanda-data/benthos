// Copyright 2025 Redpanda Data, Inc.

package pure

import (
	"bytes"
	"context"
	"errors"
	"text/template"

	"github.com/redpanda-data/benthos/v4/internal/bundle"
	"github.com/redpanda-data/benthos/v4/internal/component/interop"
	"github.com/redpanda-data/benthos/v4/public/bloblang"
	"github.com/redpanda-data/benthos/v4/public/service"
)

func tmplProcConfig() *service.ConfigSpec {
	return service.NewConfigSpec().
		Beta().
		Categories("Utility").
		Summary("Executes a Go text/template template on the message content.").
		Description(`This processor allows you to apply Go text/template templates to the structured content of messages. The template can access the message data as a structured object. Optionally, a Bloblang mapping can be applied first to transform the data before templating.

For more information on the template syntax, see https://pkg.go.dev/text/template#hdr-Actions`).
		Example(
			"Execute template",
			`This example uses a xref:components:inputs/generate.adoc[`+"`generate`"+` input] to make payload for the template.`,
			`
input:
  generate:
    count: 1
    mapping: root.foo = "bar"
  processors:
    - template:
        code: "{{ .foo }}"
`).
		Example(
			"Execute template with mapping",
			`This example uses a xref:components:inputs/generate.adoc[`+"`generate`"+` input] to make payload for the template.`,
			`
input:
  generate:
    count: 1
    mapping: root.foo = "bar"
  processors:
    - template:
        code: "{{ .value }}"
        mapping: "root.value = this.foo"
`).
		Example(
			"Execute template from file",
			`This example loads a template from a file and applies it to the message.`,
			`
input:
  generate:
    count: 1
    mapping: root.foo = "bar"
  processors:
    - template:
        code: |
          {{ template "greeting" . }}
        files: ["./templates/greeting.tmpl"]
`).
		Fields(
			service.NewStringField("code").
				Description("The template code to execute. This should be a valid Go text/template string.").
				Example("{{.name}}").
				Optional(),
			service.NewStringListField("files").
				Description("A list of file paths containing template definitions. Templates from these files will be parsed and available for execution. Glob patterns are supported, including super globs (double star).").
				Optional(),
			service.NewBloblangField("mapping").
				Description("An optional xref:guides:bloblang/about.adoc[Bloblang] mapping to apply to the message before executing the template. This allows you to transform the data structure before templating.").
				Optional(),
		)
}

func init() {
	service.MustRegisterProcessor(
		"template",
		tmplProcConfig(),
		func(conf *service.ParsedConfig, res *service.Resources) (service.Processor, error) {
			mgr := interop.UnwrapManagement(res)
			return templateFromParsed(conf, mgr)
		},
	)
}

type tmplProc struct {
	tmpl *template.Template
	exec *bloblang.Executor
}

func templateFromParsed(conf *service.ParsedConfig, mgr bundle.NewManagement) (*tmplProc, error) {
	code, err := conf.FieldString("code")
	if err != nil {
		return nil, err
	}

	files, err := conf.FieldStringList("files")
	if err != nil {
		return nil, err
	}

	if code == "" && len(files) == 0 {
		return nil, errors.New("at least one of 'code' or 'files' fields must be specified")
	}

	t := &tmplProc{tmpl: template.New("root")}
	if len(files) > 0 {
		for _, f := range files {
			if t.tmpl, err = t.tmpl.ParseGlob(f); err != nil {
				return nil, err
			}
		}
	}

	if code != "" {
		if t.tmpl, err = t.tmpl.New("code").Parse(code); err != nil {
			return nil, err
		}
	}

	if conf.Contains("mapping") {
		if t.exec, err = conf.FieldBloblang("mapping"); err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *tmplProc) Process(ctx context.Context, msg *service.Message) (service.MessageBatch, error) {
	var data any
	var err error
	if t.exec != nil {
		mapRes, err := msg.BloblangQuery(t.exec)
		if err != nil {
			return nil, err
		}

		data, err = mapRes.AsStructured()
		if err != nil {
			return nil, err
		}
	} else {
		data, err = msg.AsStructured()
		if err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	msg.SetBytes(buf.Bytes())

	return service.MessageBatch{msg}, nil
}

func (t *tmplProc) Close(ctx context.Context) error {
	return nil
}
