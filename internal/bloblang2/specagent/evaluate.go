package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bloblang2 "github.com/redpanda-data/benthos/v4/internal/bloblang2"
)

type evalResult struct {
	ID       string
	Category string
	Name     string
	Passed   bool
	Error    string
}

func cmdEvaluate(args []string) {
	fs := flag.NewFlagSet("evaluate", flag.ExitOnError)
	dir := fs.String("dir", "", "clean room base directory (from prepare)")
	mode := fs.String("mode", "both", "which clean room to evaluate: predict-output, predict-mapping, or both")
	verbose := fs.Bool("verbose", false, "show individual test results")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: specagent evaluate [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "error: --dir is required")
		fs.Usage()
		os.Exit(1)
	}

	mf, err := loadManifest(filepath.Join(*dir, "manifest.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading manifest: %v\n", err)
		os.Exit(1)
	}

	modes := resolveModes(*mode)

	var allResults []modeResults

	for _, m := range modes {
		cleanRoom := filepath.Join(*dir, m)
		var results []evalResult
		switch m {
		case "predict_output":
			results = evaluatePredictOutput(cleanRoom, mf)
		case "predict_mapping":
			results = evaluatePredictMapping(cleanRoom, mf)
		}
		allResults = append(allResults, modeResults{name: m, results: results})
	}

	if *verbose {
		for _, mr := range allResults {
			fmt.Printf("\n=== %s ===\n", mr.name)
			for _, r := range mr.results {
				status := "PASS"
				if !r.Passed {
					status = "FAIL"
				}
				fmt.Printf("  %s  %s  %s\n", status, r.ID, r.Name)
				if !r.Passed && r.Error != "" {
					for _, line := range strings.Split(r.Error, "\n") {
						fmt.Printf("         %s\n", line)
					}
				}
			}
		}
		fmt.Println()
	}

	printResultsTable(allResults)
}

func loadManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func evaluatePredictOutput(cleanRoom string, mf *manifest) []evalResult {
	var results []evalResult
	for _, entry := range mf.Tests {
		outputPath := filepath.Join(cleanRoom, "tests", entry.ID+".output.json")
		r := evalResult{
			ID:       entry.ID,
			Category: entry.Category,
			Name:     entry.Name,
		}

		raw, err := readJSONFile(outputPath)
		if err != nil {
			r.Error = fmt.Sprintf("reading output: %v", err)
			results = append(results, r)
			continue
		}

		// Parse envelope.
		env, ok := raw.(map[string]any)
		if !ok {
			r.Error = fmt.Sprintf("output is not a JSON object (got %T)", raw)
			results = append(results, r)
			continue
		}

		actualValue := env["value"]
		actualMeta, _ := env["metadata"].(map[string]any)
		if actualMeta == nil {
			actualMeta = map[string]any{}
		}

		// Both expected and actual went through json roundtrip, so all
		// numbers are float64. Compare using natural JSON semantics.
		if ok, diff := naturalJSONEqual(entry.Expected.Value, actualValue); !ok {
			r.Error = fmt.Sprintf("output mismatch: %s", diff)
			results = append(results, r)
			continue
		}

		if !entry.NoMetadataCheck {
			expectedMeta := entry.Expected.Metadata
			if expectedMeta == nil {
				expectedMeta = map[string]any{}
			}
			if ok, diff := naturalJSONEqual(any(expectedMeta), any(actualMeta)); !ok {
				r.Error = fmt.Sprintf("metadata mismatch: %s", diff)
				results = append(results, r)
				continue
			}
		}

		r.Passed = true
		results = append(results, r)
	}
	return results
}

func evaluatePredictMapping(cleanRoom string, mf *manifest) []evalResult {
	interp := &bloblang2.Interp{}
	var results []evalResult

	for _, entry := range mf.Tests {
		mappingPath := filepath.Join(cleanRoom, "tests", entry.ID+".blobl2")
		inputPath := filepath.Join(cleanRoom, "tests", entry.ID+".input.json")
		r := evalResult{
			ID:       entry.ID,
			Category: entry.Category,
			Name:     entry.Name,
		}

		// Read agent-generated mapping.
		mappingBytes, err := os.ReadFile(mappingPath)
		if err != nil {
			r.Error = fmt.Sprintf("reading mapping: %v", err)
			results = append(results, r)
			continue
		}

		// Read input (all numbers are float64 after json.Unmarshal).
		rawInput, err := readJSONFile(inputPath)
		if err != nil {
			r.Error = fmt.Sprintf("reading input: %v", err)
			results = append(results, r)
			continue
		}
		inputEnv, ok := rawInput.(map[string]any)
		if !ok {
			r.Error = "input is not a JSON object"
			results = append(results, r)
			continue
		}
		inputValue := inputEnv["value"]
		inputMeta, _ := inputEnv["metadata"].(map[string]any)
		if inputMeta == nil {
			inputMeta = map[string]any{}
		}

		// Compile mapping.
		mapping, compileErr := interp.Compile(string(mappingBytes), nil)
		if compileErr != nil {
			r.Error = fmt.Sprintf("compile error: %v", compileErr)
			results = append(results, r)
			continue
		}

		// Execute mapping.
		output, outputMeta, deleted, execErr := mapping.Exec(inputValue, inputMeta)
		if execErr != nil {
			r.Error = fmt.Sprintf("runtime error: %v", execErr)
			results = append(results, r)
			continue
		}
		if deleted {
			r.Error = "mapping deleted the message"
			results = append(results, r)
			continue
		}
		if outputMeta == nil {
			outputMeta = map[string]any{}
		}

		// Coerce interpreter output to natural JSON (all numbers → float64)
		// before comparing with expected (which is already float64 from
		// the manifest's json roundtrip).
		coercedOutput := coerceToNaturalJSON(output)
		coercedMeta := coerceToNaturalJSON(outputMeta)

		if ok, diff := naturalJSONEqual(entry.Expected.Value, coercedOutput); !ok {
			r.Error = fmt.Sprintf("output mismatch: %s", diff)
			results = append(results, r)
			continue
		}

		if !entry.NoMetadataCheck {
			expectedMeta := entry.Expected.Metadata
			if expectedMeta == nil {
				expectedMeta = map[string]any{}
			}
			if ok, diff := naturalJSONEqual(any(expectedMeta), coercedMeta); !ok {
				r.Error = fmt.Sprintf("metadata mismatch: %s", diff)
				results = append(results, r)
				continue
			}
		}

		r.Passed = true
		results = append(results, r)
	}
	return results
}

type modeResults struct {
	name    string
	results []evalResult
}

func printResultsTable(allResults []modeResults) {
	// Gather all categories in sorted order.
	catSet := map[string]struct{}{}
	for _, mr := range allResults {
		for _, r := range mr.results {
			catSet[r.Category] = struct{}{}
		}
	}
	categories := make([]string, 0, len(catSet))
	for c := range catSet {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	// Build per-category stats for each mode.
	type stats struct {
		pass, total int
	}
	modeStats := map[string]map[string]stats{}
	modeTotals := map[string]stats{}
	for _, mr := range allResults {
		catMap := map[string]stats{}
		var total stats
		for _, r := range mr.results {
			s := catMap[r.Category]
			s.total++
			if r.Passed {
				s.pass++
			}
			catMap[r.Category] = s
			total.total++
			if r.Passed {
				total.pass++
			}
		}
		modeStats[mr.name] = catMap
		modeTotals[mr.name] = total
	}

	// Determine column widths.
	catWidth := len("Category")
	for _, c := range categories {
		if len(c) > catWidth {
			catWidth = len(c)
		}
	}
	if len("TOTAL") > catWidth {
		catWidth = len("TOTAL")
	}

	colWidth := 20
	modeNames := make([]string, len(allResults))
	for i, mr := range allResults {
		modeNames[i] = mr.name
		if len(mr.name)+2 > colWidth {
			colWidth = len(mr.name) + 2
		}
	}

	// Print table.
	sep := "+" + strings.Repeat("-", catWidth+2)
	for range modeNames {
		sep += "+" + strings.Repeat("-", colWidth+2)
	}
	sep += "+"

	fmt.Println(sep)
	header := fmt.Sprintf("| %-*s", catWidth, "Category")
	for _, name := range modeNames {
		header += fmt.Sprintf(" | %-*s", colWidth, name)
	}
	header += " |"
	fmt.Println(header)
	fmt.Println(sep)

	for _, cat := range categories {
		row := fmt.Sprintf("| %-*s", catWidth, cat)
		for _, name := range modeNames {
			s := modeStats[name][cat]
			cell := formatStats(s.pass, s.total)
			row += fmt.Sprintf(" | %-*s", colWidth, cell)
		}
		row += " |"
		fmt.Println(row)
	}

	fmt.Println(sep)
	totalRow := fmt.Sprintf("| %-*s", catWidth, "TOTAL")
	for _, name := range modeNames {
		s := modeTotals[name]
		cell := formatStats(s.pass, s.total)
		totalRow += fmt.Sprintf(" | %-*s", colWidth, cell)
	}
	totalRow += " |"
	fmt.Println(totalRow)
	fmt.Println(sep)
}

func formatStats(pass, total int) string {
	if total == 0 {
		return "N/A"
	}
	pct := float64(pass) / float64(total) * 100
	return fmt.Sprintf("%5.1f%% (%d/%d)", pct, pass, total)
}
