// Copyright 2026 Redpanda Data, Inc.

package docs

import (
	"regexp"
	"strings"
)

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
}

var (
	mdXrefRx       = regexp.MustCompile(`xref:([^\s\[\]]+)\[([^\]]*)\]`)
	mdAnchorRx     = regexp.MustCompile(`<<([^,>]+)(?:,\s*([^>]*))?>>`)
	mdURLRx        = regexp.MustCompile(`(?:link:)?(https?://[^\s\[\]]+)\[([^\]]*)\]`)
	mdBoldRx       = regexp.MustCompile("(^|[\\s(])\\*([^*\\s`][^*`]*?)\\*([\\s).,:;!?]|$)")
	mdPassRx       = regexp.MustCompile(`\+\+?([^+\n]+?)\+\+?`)
	mdCondRx       = regexp.MustCompile(`^(ifdef|ifndef)::([^\[]+)\[(.*)\]$`)
	mdHeadingRx    = regexp.MustCompile(`^(={2,6}) (.+)$`)
	mdAdmonLineRx  = regexp.MustCompile(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION): (.*)$`)
	mdAdmonBlockRx = regexp.MustCompile(`^\[(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]$`)
	mdSourceRx     = regexp.MustCompile(`^\[(?:source)?,\s*([a-zA-Z0-9_+-]*)[^\]]*\]$`)
	mdBlockAttrRx  = regexp.MustCompile(`^\[[^\]]*\]$`)
	mdTabRx        = regexp.MustCompile(`^([^\s:][^:]*)::$`)
	mdDlistRx      = regexp.MustCompile(`^([^\s:][^:]*)::\s+(.+)$`)
	mdAttrRefRx    = regexp.MustCompile(`\{(nbsp|empty|sp|blank)\}`)
	mdBlankRunRx   = regexp.MustCompile(`\n{3,}`)
)

// AsciidocToMarkdown renders an AsciiDoc doc string as Markdown. page is the
// path, relative to opts.BaseURL, of the docs page the string belongs to, such
// as components/inputs/kafka, so that <<anchor>> links resolve to that page.
//
// It covers the AsciiDoc that component, field, and Bloblang docs use: env
// conditionals, xrefs, anchors, links, headings, admonitions, source and tab
// blocks, description lists, and simple tables. Anything else passes through.
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

		if inListing {
			if trimmed == listingFence {
				out = append(out, "```")
				inListing = false
				continue
			}
			if included() {
				out = append(out, line)
			}
			continue
		}

		if m := mdCondRx.FindStringSubmatch(trimmed); m != nil {
			set := false
			for _, a := range strings.FieldsFunc(m[2], func(r rune) bool { return r == ',' || r == '+' }) {
				if opts.Attributes[a] {
					set = true
				}
			}
			keep := set == (m[1] == "ifdef")
			if m[3] != "" {
				// Single-line form: ifdef::attr[content].
				if keep && included() {
					out = append(out, inline(m[3], page, opts))
				}
				continue
			}
			cond = append(cond, keep)
			continue
		}
		if trimmed == "endif::[]" {
			if len(cond) > 0 {
				cond = cond[:len(cond)-1]
			}
			continue
		}
		if !included() {
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
				for _, c := range strings.Split(strings.TrimPrefix(trimmed, "|"), "|") {
					cells = append(cells, c)
				}
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
		case trimmed == "----" || trimmed == "```" || strings.HasPrefix(trimmed, "```"):
			if strings.HasPrefix(trimmed, "```") {
				// Already a Markdown fence: copy the block as is.
				out = append(out, trimmed)
				for i++; i < len(lines); i++ {
					out = append(out, lines[i])
					if strings.TrimSpace(lines[i]) == "```" {
						break
					}
				}
				continue
			}
			inListing = true
			listingFence = trimmed
			out = append(out, "```"+pendingLang)
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

func titleCase(s string) string {
	return s[:1] + strings.ToLower(s[1:])
}

// inline converts inline AsciiDoc, leaving backtick code spans untouched.
func inline(s, page string, opts MarkdownOptions) string {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = inlineText(parts[i], page, opts)
	}
	s = strings.Join(parts, "`")
	// AsciiDoc needs $\{ to show ${, in code spans too.
	s = strings.ReplaceAll(s, `$\{`, "${")
	// Links whose text is code, such as xref:x.adoc[`x` input], span
	// backticks, so resolve them across the whole string.
	s = mdXrefRx.ReplaceAllStringFunc(s, func(x string) string {
		m := mdXrefRx.FindStringSubmatch(x)
		return link(m[2], xrefURL(m[1], opts), m[1])
	})
	s = mdURLRx.ReplaceAllStringFunc(s, func(x string) string {
		m := mdURLRx.FindStringSubmatch(x)
		return link(strings.TrimSuffix(m[2], "^"), m[1], m[1])
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
	return s
}

func inlineText(s, page string, opts MarkdownOptions) string {
	s = mdAttrRefRx.ReplaceAllStringFunc(s, func(x string) string {
		if x == "{nbsp}" || x == "{sp}" {
			return " "
		}
		return ""
	})
	s = mdPassRx.ReplaceAllString(s, "$1")
	s = mdBoldRx.ReplaceAllString(s, "$1**$2**$3")
	return s
}

func link(text, url, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		text = fallback
	}
	if url == "" {
		return text
	}
	return "[" + text + "](" + url + ")"
}

// xrefURL resolves an xref target such as components:inputs/kafka.adoc#tls or
// streaming:manage:x.adoc to a docs site URL, or "" when it can't.
func xrefURL(target string, opts MarkdownOptions) string {
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
		parts = []string{"ROOT", parts[0]}
	}
	if base == "" {
		return ""
	}
	module, path := parts[0], strings.TrimSuffix(parts[1], ".adoc")
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
