package syntax

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fuzzMaxInputSize caps fuzz input length; bigger inputs are skipped because
// they rarely add coverage that smaller inputs don't already hit.
const fuzzMaxInputSize = 16 * 1024

// curatedFuzzSeeds covers a broad cross-section of V2 syntax in compact form.
var curatedFuzzSeeds = []string{
	``,
	`output = 42`,
	`output = "hello"`,
	`output = -3.14`,
	`output.x = true`,
	`output.x = null`,
	`output = "line\n\ttab"`,
	`output = "uni é"`,
	"output = `raw`",
	`output = [1, 2, 3]`,
	`output = {"a": 1, "b": 2}`,
	`output = [1, [2, [3, [4]]]]`,
	`output = 1 + 2 * 3 - 4 / 5 % 6`,
	`output = a && b || !c`,
	`output = (1 == 2) != (3 < 4) && (5 >= 6)`,
	`output = -input.value`,
	`output = input.foo.bar?.baz[0]?[1]`,
	`output = input@.key`,
	`$x = input.value`,
	`output = uuid_v4()`,
	`output = input.uppercase()`,
	`output = input.map(x -> x * 2)`,
	`output = input.fold(0, (acc, x) -> acc + x)`,
	`output = math::double(5)`,
	`output = foo(a: 1, b: 2)`,
	"output = input.map(x -> {\n  $y = x * 2\n  $y + 1\n})",
	"output = if input.x > 0 { \"big\" } else { \"small\" }",
	"output = match input {\n  this > 0 => \"pos\"\n  _ => \"other\"\n}",
	"map double { this * 2 }\noutput = input.apply(double)",
	`import "lib.blobl" as lib`,
	"output.a = 1\noutput.b = 2\noutput.c = 3",
	"# leading\noutput = 1 # trailing\n\n# blank above",
	`output = deleted()`,
	`output = throw("boom")`,
	`output = void()`,
}

// adversarialFuzzSeeds are pathological inputs targeting known edge cases.
var adversarialFuzzSeeds = []string{
	"output = " + strings.Repeat("(", 200) + "1" + strings.Repeat(")", 200),
	"output = " + strings.Repeat("input.x.", 100) + "y",
	"\x00\x00\x00",
	"\xef\xbb\xbfoutput = 1",
	"output = \"\\uD800\"",
	"output = \"unterminated",
	"output = `unterminated raw",
	"output = #incomplete",
	"output = 1\r\n2\r\n3",
	"output = 0x",
	"output = 1.",
	"output = .1",
	"output = $",
	"output = ::",
	"output = []",
	"output = {}",
}

// loadFuzzCorpus returns the deduplicated full seed corpus: curated +
// adversarial + spec-derived mappings.
func loadFuzzCorpus(tb testing.TB) []string {
	tb.Helper()
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range curatedFuzzSeeds {
		add(s)
	}
	for _, s := range adversarialFuzzSeeds {
		add(s)
	}
	for _, s := range loadSpecMappings(tb) {
		add(s)
	}
	return out
}

// loadSpecMappings walks the spec/tests directory and extracts every
// `mapping:` field from each YAML test file (top-level and nested within
// `cases:`). Best-effort: returns whatever it can parse, silently skipping
// files it can't.
func loadSpecMappings(tb testing.TB) []string {
	tb.Helper()
	specDir := filepath.Join("..", "..", "..", "spec", "tests")
	if _, err := os.Stat(specDir); err != nil {
		tb.Logf("spec dir not found at %s; skipping spec corpus", specDir)
		return nil
	}
	var mappings []string
	walkErr := filepath.WalkDir(specDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var f struct {
			Tests []struct {
				Mapping string `yaml:"mapping"`
				Cases   []struct {
					Mapping string `yaml:"mapping"`
				} `yaml:"cases"`
			} `yaml:"tests"`
		}
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil
		}
		for _, t := range f.Tests {
			if t.Mapping != "" {
				mappings = append(mappings, t.Mapping)
			}
			for _, c := range t.Cases {
				if c.Mapping != "" {
					mappings = append(mappings, c.Mapping)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		tb.Logf("walking spec dir: %v", walkErr)
	}
	return mappings
}
