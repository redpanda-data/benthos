package translator

import (
	"fmt"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/migrator/v1ast"
)

// siteKey identifies an import site for de-duplication and lookup.
// parentKey is the canonical key of the file the import appears in
// (empty for the main V1 source). importPath is the path string as
// written in the import statement.
type siteKey struct {
	parentKey  string
	importPath string
}

// fileSet is the result of walking the closure of imports rooted at
// the main V1 source. contents is keyed by canonical key; siteIndex
// maps each visited (parentKey, importPath) pair to the canonical
// key the resolver assigned. Unresolved imports are recorded on
// unresolved so the translator can surface RuleImportStatement
// Unsupported changes at the appropriate sites.
type fileSet struct {
	contents   map[string]string
	siteIndex  map[siteKey]string
	unresolved map[siteKey]struct{}
}

func newFileSet() *fileSet {
	return &fileSet{
		contents:   map[string]string{},
		siteIndex:  map[siteKey]string{},
		unresolved: map[siteKey]struct{}{},
	}
}

// buildFileSet walks the closure of imports rooted at the supplied
// main V1 source. Pre-populated entries in opts.Files take precedence:
// any importPath whose string appears as a key in Files is treated as
// already resolved with the path string as its canonical key. Imports
// not satisfied by Files consult opts.FileResolver, if set.
//
// Imports that cannot be resolved (no Files entry, no resolver, or
// resolver returns ok=false) are recorded on the returned fileSet's
// unresolved map; the translator surfaces them as RuleImportStatement
// Unsupported changes during translation.
//
// A V1 parse error inside an imported file is fatal — the caller is
// supplying broken content.
func buildFileSet(mainSource string, opts Options) (*fileSet, error) {
	fs := newFileSet()

	// Seed contents with any pre-populated Files. Each entry is treated
	// as already canonicalised with the map key as its canonical key.
	for k, v := range opts.Files {
		fs.contents[k] = v
	}

	// BFS over the import graph. queue entries are (parentKey, source)
	// pairs — the source whose imports we still need to discover.
	type queueEntry struct {
		parentKey string
		source    string
	}
	queue := []queueEntry{{parentKey: "", source: mainSource}}
	visited := map[string]struct{}{}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		prog, err := v1ast.Parse(entry.source)
		if err != nil {
			if entry.parentKey == "" {
				// Main source parse error — return it; the caller's
				// migrateSource will attempt the same parse and produce
				// a more accurate error message there.
				return fs, nil
			}
			return nil, fmt.Errorf("parsing imported file %q: %w", entry.parentKey, err)
		}

		for _, stmt := range prog.Stmts {
			var pathStr string
			switch s := stmt.(type) {
			case *v1ast.ImportStmt:
				lit, ok := s.Path.(*v1ast.Literal)
				if !ok {
					continue
				}
				pathStr = lit.Str
			case *v1ast.FromStmt:
				lit, ok := s.Path.(*v1ast.Literal)
				if !ok {
					continue
				}
				pathStr = lit.Str
			default:
				continue
			}
			site := siteKey{parentKey: entry.parentKey, importPath: pathStr}

			canonical, content, resolved, consulted := resolveImport(opts, site)
			if !resolved {
				if consulted {
					// FileResolver was set and explicitly declined this
					// path — surface as unresolved so translateImport
					// can flag it. Imports with no resolver and no
					// Files entry fall through to the legacy path
					// (V2 import statement emitted verbatim; the V2
					// sanity-check Parse surfaces the missing file).
					fs.unresolved[site] = struct{}{}
				}
				continue
			}
			fs.siteIndex[site] = canonical
			if _, seen := visited[canonical]; seen {
				continue
			}
			visited[canonical] = struct{}{}
			if _, exists := fs.contents[canonical]; !exists {
				fs.contents[canonical] = content
			}
			queue = append(queue, queueEntry{parentKey: canonical, source: content})
		}
	}

	return fs, nil
}

// resolveImport applies the Files-then-FileResolver precedence to a
// single import site. Returns the canonical key, the V1 content,
// whether the import was resolved, and whether the resolver was
// consulted (i.e. FileResolver was set and called). The consulted
// flag distinguishes an explicit "could not resolve" answer from
// "no resolver configured" — the former is surfaced as Unsupported,
// the latter falls through to the legacy "emit V2 import verbatim"
// path so callers without a resolver retain the old behaviour.
func resolveImport(opts Options, site siteKey) (canonical, content string, ok, consulted bool) {
	if c, exists := opts.Files[site.importPath]; exists {
		return site.importPath, c, true, false
	}
	if opts.FileResolver == nil {
		return "", "", false, false
	}
	canonical, content, ok = opts.FileResolver(site.parentKey, site.importPath)
	if !ok {
		return "", "", false, true
	}
	return canonical, content, true, true
}
