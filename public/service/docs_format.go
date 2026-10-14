// Copyright 2026 Redpanda Data, Inc.

package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// DocsAttributeCloud is the AsciiDoc attribute that marks a reader as running
// Redpanda Connect inside Redpanda Cloud. The Cloud docs site sets it when it
// builds, and a distribution that runs in Cloud declares it on its schema
// with SetDocsAttributes, so that doc strings wrapped in
// ifndef::env-cloud[] ... endif::[] or ifdef::env-cloud[] ... endif::[] show
// the same text on the Cloud docs site and in that distribution's output.
// It is the only attribute doc strings may test; see KnownDocsAttributes.
const DocsAttributeCloud = "env-cloud"

// KnownDocsAttributes returns the AsciiDoc attributes doc strings may test
// with ifdef and ifndef, each with the sentence explaining when it is set.
func KnownDocsAttributes() map[string]string {
	out := make(map[string]string, len(docs.KnownDocsAttributes))
	for k, v := range docs.KnownDocsAttributes {
		out[k] = v
	}
	return out
}

// SetDocsAttributes declares the AsciiDoc attributes that are set for readers
// of this distribution, such as DocsAttributeCloud for a build that runs in
// Redpanda Cloud. They decide which ifdef and ifndef blocks in doc strings
// this distribution shows when it renders them as Markdown, so the schema
// that defines a distribution's components is also where its attributes
// live, and the binaries and the docs site cannot disagree.
func (s *ConfigSchema) SetDocsAttributes(attrs ...string) *ConfigSchema {
	s.docsAttributes = append([]string{}, attrs...)
	return s
}

// DocsAttributes returns the attributes set with SetDocsAttributes.
func (s *ConfigSchema) DocsAttributes() []string {
	return append([]string{}, s.docsAttributes...)
}

// DocsMarkdownOptions configures how MarshalJSONV0Markdown resolves links.
type DocsMarkdownOptions struct {
	// DocumentationURL is the base URL of this distribution's Connect docs,
	// for example https://docs.redpanda.com/connect. Links render as plain
	// text when it is empty.
	DocumentationURL string

	// ComponentURLs maps other docs site components named in
	// component-qualified xrefs, such as streaming in
	// xref:streaming:manage:x.adoc[], to their base URLs.
	ComponentURLs map[string]string
}

// MarshalJSONV0Markdown is MarshalJSONV0 with every doc string rendered from
// AsciiDoc to Markdown for consumers other than the docs site: xrefs and
// anchors become absolute links, links to components this schema lacks
// become text, and ifdef and ifndef conditionals are resolved for the
// attributes set with SetDocsAttributes. The document's docs_format field
// says "markdown". It fails when a doc string has a malformed conditional,
// naming each one, because a renderer would otherwise drop text silently.
func (s *ConfigSchema) MarshalJSONV0Markdown(opts DocsMarkdownOptions) ([]byte, error) {
	full := s.fullSchema()
	attrs := map[string]bool{}
	for _, a := range s.docsAttributes {
		attrs[a] = true
	}
	problems := full.RenderMarkdown(docs.MarkdownOptions{
		BaseURL:       opts.DocumentationURL,
		Attributes:    attrs,
		ComponentURLs: opts.ComponentURLs,
	})
	if len(problems) > 0 {
		return nil, errors.New("doc strings have malformed conditionals:\n  " + strings.Join(problems, "\n  "))
	}
	return json.Marshal(full)
}

// LintDocsConditionals reports the ways a doc string's ifdef, ifndef and
// endif directives would surprise one of their renderers, or nothing when
// the string follows the one supported form: a directive on its own line at
// column 0 testing a known attribute, the whole paragraph inside, a blank
// line before and after, and endif::[] to close. Tests over a distribution's
// schema call it so a bad conditional fails at build time rather than
// dropping text for readers.
func LintDocsConditionals(s string) []string {
	return docs.LintConditionals(s)
}
