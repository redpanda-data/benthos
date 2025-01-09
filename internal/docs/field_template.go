// Copyright 2025 Redpanda Data, Inc.

package docs

// DeprecatedFieldsTemplate is an old (now unused) template for generating
// documentation. It has been replace with public methods for exporting template
// data, allowing you to use whichever template suits your needs.
// TODO: V5 Remove this
func DeprecatedFieldsTemplate(lintableExamples bool) string {
	// Use trailing whitespace below to render line breaks in Asciidoc
	return `{{define "field_docs" -}}
{{range $i, $field := .Fields -}}
=== ` + "`{{$field.FullName}}`" + `

{{$field.Description}}
{{if $field.IsSecret -}}

[CAUTION]
====
This field contains sensitive information that usually shouldn't be added to a config directly, read our xref:configuration:secrets.adoc[secrets page for more info].
====

{{end -}}
{{if $field.IsInterpolated -}}
This field supports xref:configuration:interpolation.adoc#bloblang-queries[interpolation functions].
{{end}}

*Type*: ` + "`{{$field.Type}}`" + `

{{if gt (len $field.DefaultMarshalled) 0}}*Default*: ` + "`{{$field.DefaultMarshalled}}`" + `
{{end -}}
{{if gt (len $field.Version) 0}}Requires version {{$field.Version}} or newer
{{end -}}
{{if gt (len $field.AnnotatedOptions) 0}}
|===
| Option | Summary

{{range $j, $option := $field.AnnotatedOptions -}}
| ` + "`{{index $option 0}}`" + `
| {{index $option 1}}
{{end}}
|===
{{else if gt (len $field.Options) 0}}
Options:
{{range $j, $option := $field.Options -}}
{{if ne $j 0}}, {{end}}` + "`{{$option}}`" + `
{{end}}.
{{end}}
{{if gt (len $field.Examples) 0 -}}
` + "```yml" + `
# Examples

{{range $j, $example := $field.ExamplesMarshalled -}}
{{if ne $j 0}}
{{end}}{{$example}}{{end -}}
` + "```" + `

{{end -}}
{{end -}}
{{end -}}`
}
