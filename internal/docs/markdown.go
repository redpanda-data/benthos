// Copyright 2026 Redpanda Data, Inc.

package docs

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// KnownDocsAttributes is the vocabulary of AsciiDoc attributes that doc
// strings may test with ifdef and ifndef. Each maps to the sentence that
// explains when it is set, for lint messages and documentation. Which
// distribution sets which attribute is decided where that distribution's
// schema is built, and the docs site builds set the same attributes.
var KnownDocsAttributes = map[string]string{
	"env-cloud": "the reader runs Redpanda Connect inside Redpanda Cloud; the Cloud docs site and the Cloud binaries set it",
}

// MarkdownOptions controls how AsciiDoc doc strings render as Markdown for
// consumers other than the docs site, such as CLI output, JSON schemas, and
// editors.
type MarkdownOptions struct {
	// BaseURL is the docs site URL that xrefs resolve against, for example
	// https://docs.redpanda.com/connect. Links render as text when it's empty.
	BaseURL string

	// Attributes are the AsciiDoc attributes treated as set when resolving
	// ifdef and ifndef conditionals, for example env-cloud in a binary that
	// runs in Redpanda Cloud.
	Attributes map[string]bool

	// ComponentURLs maps other docs site components, as named in a
	// component-qualified xref such as xref:streaming:manage:x.adoc[], to their
	// base URLs. An xref to a component missing from the map renders as text.
	ComponentURLs map[string]string

	// HasComponent reports whether the distribution being rendered includes
	// the Connect component at components/<typeDir>/<name>. When set, an xref
	// to a component it lacks renders as text rather than a link, the same
	// rule the docs site applies, so doc strings never need a conditional
	// just to hide a link.
	HasComponent func(typeDir, name string) bool
}

var (
	// Link text may contain \] to escape a bracket.
	mdLabel        = `((?:[^\]\\]|\\.)*)`
	mdXrefRx       = regexp.MustCompile(`xref:([^\s\[\]]+)\[` + mdLabel + `\]`)
	mdURLRx        = regexp.MustCompile(`(?:link:)?(https?://[^\s\[\]]+)\[` + mdLabel + `\]`)
	mdAnchorRx     = regexp.MustCompile(`<<([^,>\s]+)(?:,\s*([^>]*))?>>`)
	mdLinkAttrRx   = regexp.MustCompile(`,\s*(?:window|role|opts|id|title)=.*$`)
	mdBoldRx       = regexp.MustCompile("(^|[\\s(])\\*([^*\\s`](?:[^*`]*?[^*\\s`])?)\\*([\\s).,:;!?]|$)")
	mdPassDoubleRx = regexp.MustCompile(`\+\+([^+\n]+?)\+\+`)
	mdPassSingleRx = regexp.MustCompile(`(^|[^\w+])\+([^+\s](?:[^+\n]*?[^+\s])?)\+($|[^\w+])`)
	mdCondRx       = regexp.MustCompile(`^(ifdef|ifndef)::([^\[\]]+)\[(.*)\]$`)
	mdEndifRx      = regexp.MustCompile(`^endif::[^\[\]]*\[\]$`)
	mdIfevalRx     = regexp.MustCompile(`^ifeval::\[`)
	mdHeadingRx    = regexp.MustCompile(`^(={2,6}) (.+)$`)
	mdAdmonLineRx  = regexp.MustCompile(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION): (.*)$`)
	mdAdmonBlockRx = regexp.MustCompile(`^\[(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]$`)
	mdSourceRx     = regexp.MustCompile(`^\[(?:source)?,\s*([a-zA-Z0-9_+-]*)[^\]]*\]$`)
	mdBlockAttrRx  = regexp.MustCompile(`^\[[^\]]*\]$`)
	mdTabRx        = regexp.MustCompile(`^([^\s:][^:]*)::$`)
	mdDlistRx      = regexp.MustCompile(`^([^\s:][^:]*)::\s+(.+)$`)
	mdOlistRx      = regexp.MustCompile(`^(\.{1,5}) (.+)$`)
	mdUlistRx      = regexp.MustCompile(`^(\*{1,5}) (.+)$`)
	mdCellSpecRx   = regexp.MustCompile(`(^|\s)(?:\d+(?:\.\d+)?\+|\.\d+\+)?(?:\d+\*)?[<^>]?(?:\.[<^>])?[adehlmsv]?\|`)
	mdAttrRefRx    = regexp.MustCompile(`\{(nbsp|sp|empty|blank|plus|vbar|zwsp|startsb|endsb)\}`)
	mdBlankRunRx   = regexp.MustCompile(`\n{3,}`)
	mdPlaceholder  = "\x00"
)

var mdAttrRefs = map[string]string{
	"nbsp": " ", "sp": " ", "empty": "", "blank": "", "plus": "+", "vbar": "|", "zwsp": "", "startsb": "[", "endsb": "]",
}

// LintConditionals reports the ways a doc string's ifdef, ifndef and endif
// directives would surprise one of their renderers. Asciidoctor, the docs
// generator, and AsciidocToMarkdown each read directives slightly
// differently, so doc strings use one form that all of them agree on: a
// directive on its own line at column 0, testing one attribute from
// KnownDocsAttributes, with the whole paragraph inside and a blank line
// before and after the block. The result is empty when the string is fine.
func LintConditionals(s string) []string {
	var problems []string
	lines := strings.Split(s, "\n")
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isDirective := strings.HasPrefix(trimmed, "ifdef::") || strings.HasPrefix(trimmed, "ifndef::") ||
			strings.HasPrefix(trimmed, "endif::") || strings.HasPrefix(trimmed, "ifeval::")
		if !isDirective {
			continue
		}
		if trimmed != line {
			problems = append(problems, fmt.Sprintf("line %d: %q must start at column 0 to be a directive", i+1, trimmed))
			continue
		}
		if mdIfevalRx.MatchString(trimmed) {
			problems = append(problems, fmt.Sprintf("line %d: ifeval is not supported in doc strings", i+1))
			continue
		}
		if m := mdCondRx.FindStringSubmatch(trimmed); m != nil {
			if m[3] != "" {
				problems = append(problems, fmt.Sprintf("line %d: use the block form (%s::%s[] on its own line, the paragraph, then endif::[]), not %s::%s[text]", i+1, m[1], m[2], m[1], m[2]))
			}
			for _, a := range strings.FieldsFunc(m[2], func(r rune) bool { return r == ',' || r == '+' }) {
				if _, ok := KnownDocsAttributes[a]; !ok {
					problems = append(problems, fmt.Sprintf("line %d: unknown attribute %q; known attributes: %s", i+1, a, strings.Join(knownDocsAttributeNames(), ", ")))
				}
			}
			if m[3] == "" {
				depth++
				if i > 0 && strings.TrimSpace(lines[i-1]) != "" {
					problems = append(problems, fmt.Sprintf("line %d: put a blank line before %s so the paragraph before it stays separate", i+1, trimmed))
				}
			}
			continue
		}
		if mdEndifRx.MatchString(trimmed) {
			if depth == 0 {
				problems = append(problems, fmt.Sprintf("line %d: endif without a matching ifdef or ifndef", i+1))
			} else {
				depth--
			}
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				problems = append(problems, fmt.Sprintf("line %d: put a blank line after %s so the paragraph after it stays separate", i+1, trimmed))
			}
			continue
		}
		problems = append(problems, fmt.Sprintf("line %d: malformed directive %q", i+1, trimmed))
	}
	if depth > 0 {
		problems = append(problems, fmt.Sprintf("%d ifdef or ifndef block(s) never closed with endif::[]; every renderer would drop the rest of the text", depth))
	}
	return problems
}

func knownDocsAttributeNames() []string {
	names := make([]string, 0, len(KnownDocsAttributes))
	for k := range KnownDocsAttributes {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// AsciidocToMarkdown renders an AsciiDoc doc string as Markdown. page is the
// path, relative to opts.BaseURL, of the docs page the string belongs to, such
// as components/inputs/kafka, so that <<anchor>> links and module-less xrefs
// resolve against it.
//
// It covers the AsciiDoc that component, field, and Bloblang docs use: env
// conditionals, xrefs, anchors, links, headings, admonitions, lists, source
// and tab blocks, description lists, and simple tables. Anything else passes
// through. Conditionals follow the AsciiDoc preprocessor: a directive counts
// only at column 0, a comma list means any attribute, a plus list means all.
func AsciidocToMarkdown(src, page string, opts MarkdownOptions) string {
	var out []string
	lines := strings.Split(src, "\n")

	// Conditional stack: each entry says whether that level is included.
	var cond []bool
	included := func() bool {
		for _, c := range cond {
			if !c {
				return false
			}
		}
		return true
	}

	var (
		inListing    bool
		listingFence string
		pendingLang  string
		pendingAdmon string
		inQuote      bool
		inTabs       bool
		inTable      bool
		tableSep     string
		tableRows    [][]string
		tableHeader  bool
		pendingTable string
	)

	flushTable := func() {
		if len(tableRows) == 0 {
			return
		}
		cols := len(tableRows[0])
		row := func(cells []string) string {
			for len(cells) < cols {
				cells = append(cells, "")
			}
			return "| " + strings.Join(cells[:cols], " | ") + " |"
		}
		out = append(out, row(tableRows[0]))
		sep := make([]string, cols)
		for i := range sep {
			sep[i] = "---"
		}
		out = append(out, "| "+strings.Join(sep, " | ")+" |")
		for _, r := range tableRows[1:] {
			out = append(out, row(r))
		}
		tableRows = nil
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Preprocessor directives come first and count only at column 0,
		// wherever they appear, as in Asciidoctor.
		if m := mdCondRx.FindStringSubmatch(line); m != nil {
			keep := attributesSatisfied(m[1], m[2], opts.Attributes)
			if m[3] != "" {
				// Single-line form, which the lint rejects but Asciidoctor
				// honours.
				if keep && included() {
					out = append(out, inline(m[3], page, opts))
				}
				continue
			}
			cond = append(cond, keep)
			continue
		}
		if mdEndifRx.MatchString(line) {
			if len(cond) > 0 {
				cond = cond[:len(cond)-1]
			}
			continue
		}
		if !included() {
			continue
		}

		if inListing {
			if trimmed == listingFence {
				out = append(out, "```")
				inListing = false
				continue
			}
			out = append(out, line)
			continue
		}

		if inTable {
			if trimmed == "|===" {
				flushTable()
				inTable = false
				continue
			}
			if trimmed == "" {
				continue
			}
			var cells []string
			if tableSep == ":" {
				cells = strings.Split(trimmed, ":")
			} else {
				// Drop cell specifiers such as a| or 2+| before splitting.
				plain := mdCellSpecRx.ReplaceAllString(trimmed, "$1|")
				cells = strings.Split(strings.TrimPrefix(plain, "|"), "|")
			}
			for j := range cells {
				cells[j] = strings.ReplaceAll(inline(strings.TrimSpace(cells[j]), page, opts), "|", `\|`)
			}
			// In PSV tables a row may span lines, one cell per line.
			if tableSep != ":" && len(tableRows) > 0 && !tableHeader && len(tableRows[len(tableRows)-1]) < len(tableRows[0]) {
				tableRows[len(tableRows)-1] = append(tableRows[len(tableRows)-1], cells...)
			} else {
				tableRows = append(tableRows, cells)
			}
			tableHeader = false
			continue
		}

		switch {
		case trimmed == "----" || strings.HasPrefix(trimmed, "```"):
			inListing = true
			if strings.HasPrefix(trimmed, "```") {
				// Already a Markdown fence.
				listingFence = "```"
				out = append(out, trimmed)
			} else {
				listingFence = "----"
				out = append(out, "```"+pendingLang)
			}
			pendingLang = ""
			continue
		case trimmed == "|===":
			inTable = true
			tableSep = "|"
			if strings.Contains(pendingTable, "dsv") {
				tableSep = ":"
			}
			tableHeader = true
			pendingTable = ""
			continue
		case mdSourceRx.MatchString(trimmed):
			pendingLang = mdSourceRx.FindStringSubmatch(trimmed)[1]
			continue
		case mdAdmonBlockRx.MatchString(trimmed):
			pendingAdmon = mdAdmonBlockRx.FindStringSubmatch(trimmed)[1]
			continue
		case trimmed == "[tabs]":
			inTabs = true
			continue
		case trimmed == "======" && inTabs:
			if len(out) > 0 && out[len(out)-1] == "" {
				inTabs = false
			}
			continue
		case trimmed == "====":
			if inQuote {
				inQuote = false
			} else {
				inQuote = true
				if pendingAdmon != "" {
					out = append(out, "> **"+titleCase(pendingAdmon)+":**")
					pendingAdmon = ""
				}
			}
			continue
		case trimmed == "--" || trimmed == "+":
			continue
		case mdBlockAttrRx.MatchString(trimmed):
			pendingTable = trimmed
			continue
		case strings.HasPrefix(trimmed, ".") && len(trimmed) > 1 && trimmed[1] != '.' && trimmed[1] != ' ':
			// Block title.
			out = append(out, "**"+inline(trimmed[1:], page, opts)+"**")
			continue
		}

		if inTabs {
			if m := mdTabRx.FindStringSubmatch(trimmed); m != nil {
				out = append(out, "**"+m[1]+"**")
				continue
			}
		}

		text := line
		if m := mdHeadingRx.FindStringSubmatch(trimmed); m != nil {
			text = strings.Repeat("#", len(m[1])) + " " + inline(m[2], page, opts)
		} else if m := mdAdmonLineRx.FindStringSubmatch(trimmed); m != nil {
			text = "> **" + titleCase(m[1]) + ":** " + inline(m[2], page, opts)
		} else if m := mdDlistRx.FindStringSubmatch(trimmed); m != nil {
			text = "**" + inline(m[1], page, opts) + "**: " + inline(m[2], page, opts)
		} else if m := mdOlistRx.FindStringSubmatch(trimmed); m != nil {
			text = strings.Repeat("   ", len(m[1])-1) + "1. " + inline(m[2], page, opts)
		} else if m := mdUlistRx.FindStringSubmatch(trimmed); m != nil {
			text = strings.Repeat("  ", len(m[1])-1) + "- " + inline(m[2], page, opts)
		} else {
			text = inline(line, page, opts)
		}
		if inQuote {
			text = strings.TrimRight("> "+text, " ")
		}
		out = append(out, text)
	}
	flushTable()
	// Dropped conditionals and block delimiters leave runs of blank lines.
	return mdBlankRunRx.ReplaceAllString(strings.Join(out, "\n"), "\n\n")
}

// attributesSatisfied applies the preprocessor rule: a comma-separated list
// is satisfied when any attribute is set, a plus-separated list when all are.
func attributesSatisfied(directive, list string, attrs map[string]bool) bool {
	var set bool
	if strings.Contains(list, "+") {
		set = true
		for _, a := range strings.Split(list, "+") {
			if !attrs[a] {
				set = false
			}
		}
	} else {
		for _, a := range strings.Split(list, ",") {
			if attrs[a] {
				set = true
			}
		}
	}
	return set == (directive == "ifdef")
}

func titleCase(s string) string {
	return s[:1] + strings.ToLower(s[1:])
}

// inline converts inline AsciiDoc, leaving backtick code spans untouched.
func inline(s, page string, opts MarkdownOptions) string {
	// Protect ++literal++ passthroughs from every other rule.
	var literals []string
	s = mdPassDoubleRx.ReplaceAllStringFunc(s, func(x string) string {
		literals = append(literals, mdPassDoubleRx.FindStringSubmatch(x)[1])
		return mdPlaceholder
	})
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = inlineText(parts[i])
	}
	s = strings.Join(parts, "`")
	// Links whose text is code, such as xref:x.adoc[`x` input], span
	// backticks, so resolve them across the whole string.
	s = mdXrefRx.ReplaceAllStringFunc(s, func(x string) string {
		m := mdXrefRx.FindStringSubmatch(x)
		return link(m[2], xrefURL(m[1], page, opts), xrefLabel(m[1]))
	})
	s = mdURLRx.ReplaceAllStringFunc(s, func(x string) string {
		m := mdURLRx.FindStringSubmatch(x)
		return link(m[2], m[1], m[1])
	})
	s = mdAnchorRx.ReplaceAllStringFunc(s, func(x string) string {
		m := mdAnchorRx.FindStringSubmatch(x)
		text := strings.TrimSpace(m[2])
		if text == "" {
			text = "`" + m[1] + "`"
		}
		url := ""
		if opts.BaseURL != "" && page != "" {
			url = strings.TrimSuffix(opts.BaseURL, "/") + "/" + page + "/#" + m[1]
		}
		return link(text, url, m[1])
	})
	// AsciiDoc needs $\{ and \{ to show a brace, in code spans too.
	s = strings.ReplaceAll(s, `\{`, "{")
	s = mdAttrRefRx.ReplaceAllStringFunc(s, func(x string) string {
		return mdAttrRefs[x[1:len(x)-1]]
	})
	for _, lit := range literals {
		s = strings.Replace(s, mdPlaceholder, lit, 1)
	}
	return s
}

func inlineText(s string) string {
	s = mdPassSingleRx.ReplaceAllString(s, "$1$2$3")
	s = mdBoldRx.ReplaceAllString(s, "$1**$2**$3")
	return s
}

// link renders Markdown for a link whose AsciiDoc text may carry \] escapes,
// a trailing ^ (open in a new window), or link attributes after a comma.
func link(text, url, fallback string) string {
	text = mdLinkAttrRx.ReplaceAllString(text, "")
	text = strings.TrimSuffix(strings.TrimSpace(text), "^")
	text = strings.TrimSpace(strings.ReplaceAll(text, `\]`, "]"))
	if text == "" {
		text = fallback
	}
	if url == "" {
		return text
	}
	return "[" + text + "](" + url + ")"
}

// xrefLabel derives readable link text for an xref with none, from the
// fragment if there is one, else the page name: configuration:error_handling
// becomes "error handling".
func xrefLabel(target string) string {
	label := target
	if i := strings.Index(target, "#"); i >= 0 && i+1 < len(target) {
		label = target[i+1:]
	} else {
		label = strings.TrimSuffix(label, ".adoc")
		if i := strings.LastIndexAny(label, ":/"); i >= 0 {
			label = label[i+1:]
		}
	}
	label = strings.NewReplacer("_", " ", "-", " ").Replace(label)
	if label == "about" || label == "index" || label == "" {
		return "the documentation"
	}
	return label
}

// xrefURL resolves an xref target such as components:inputs/kafka.adoc#tls or
// streaming:manage:x.adoc to a docs site URL, or "" when it can't, or when
// the target is a Connect component the rendered distribution lacks.
func xrefURL(target, page string, opts MarkdownOptions) string {
	frag := ""
	if i := strings.Index(target, "#"); i >= 0 {
		target, frag = target[:i], target[i:]
	}
	if i := strings.Index(target, "@"); i >= 0 {
		target = target[i+1:]
	}
	base := opts.BaseURL
	parts := strings.Split(target, ":")
	switch len(parts) {
	case 3:
		base = opts.ComponentURLs[parts[0]]
		parts = parts[1:]
	case 1:
		// A module-less xref is relative to the page's own module.
		module := "ROOT"
		if i := strings.Index(page, "/"); i >= 0 {
			module = page[:i]
		}
		parts = []string{module, parts[0]}
	}
	if base == "" {
		return ""
	}
	module, path := parts[0], strings.TrimSuffix(parts[1], ".adoc")
	if module == "components" && opts.HasComponent != nil {
		if typeDir, name, ok := strings.Cut(path, "/"); ok && name != "about" && !opts.HasComponent(typeDir, name) {
			return ""
		}
	}
	path = strings.TrimSuffix(path, "/index")
	if path == "index" {
		path = ""
	}
	url := strings.TrimSuffix(base, "/") + "/"
	if module != "ROOT" {
		url += module + "/"
	}
	if path != "" {
		url += path + "/"
	}
	return url + frag
}
