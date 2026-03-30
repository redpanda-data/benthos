package agentexam

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// LogResult writes a single result line to w. Pass results show "PASS", others
// show "FAIL" with the error on indented lines below. Useful for streaming
// results during scoring.
func LogResult(w io.Writer, r Result) {
	status := "PASS"
	if r.Score < 1 {
		status = "FAIL"
	}
	fmt.Fprintf(w, "  %s  %s  %s\n", status, r.ID, r.Name)
	if r.Score < 1 && r.Error != "" {
		for _, line := range strings.Split(r.Error, "\n") {
			fmt.Fprintf(w, "         %s\n", line)
		}
	}
}

// PrintTable writes a formatted results table to w, grouped by Result.Group.
// Results with an empty Group are listed under "(ungrouped)".
func PrintTable(w io.Writer, results []Result) {
	PrintComparisonTable(w, map[string][]Result{"results": results})
}

// PrintComparisonTable writes a multi-column results table to w. Each key in
// runs becomes a column, allowing side-by-side comparison of multiple exam
// runs.
func PrintComparisonTable(w io.Writer, runs map[string][]Result) {
	type stats struct {
		score float64
		total int
	}

	// Collect all groups and run names.
	groupSet := map[string]struct{}{}
	for _, results := range runs {
		for _, r := range results {
			g := r.Group
			if g == "" {
				g = "(ungrouped)"
			}
			groupSet[g] = struct{}{}
		}
	}

	groups := make([]string, 0, len(groupSet))
	for g := range groupSet {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	runNames := make([]string, 0, len(runs))
	for name := range runs {
		runNames = append(runNames, name)
	}
	sort.Strings(runNames)

	// Build per-group stats for each run.
	runStats := map[string]map[string]stats{}
	runTotals := map[string]stats{}
	for name, results := range runs {
		groupMap := map[string]stats{}
		var total stats
		for _, r := range results {
			g := r.Group
			if g == "" {
				g = "(ungrouped)"
			}
			s := groupMap[g]
			s.total++
			s.score += r.Score
			groupMap[g] = s
			total.total++
			total.score += r.Score
		}
		runStats[name] = groupMap
		runTotals[name] = total
	}

	// Determine column widths.
	groupWidth := len("Group")
	for _, g := range groups {
		if len(g) > groupWidth {
			groupWidth = len(g)
		}
	}
	if len("TOTAL") > groupWidth {
		groupWidth = len("TOTAL")
	}

	colWidth := 20
	for _, name := range runNames {
		if len(name)+2 > colWidth {
			colWidth = len(name) + 2
		}
	}

	// Build separator line.
	var sepBuf strings.Builder
	sepBuf.WriteString("+" + strings.Repeat("-", groupWidth+2))
	for range runNames {
		sepBuf.WriteString("+" + strings.Repeat("-", colWidth+2))
	}
	sepBuf.WriteString("+")
	sep := sepBuf.String()

	// Print table.
	fmt.Fprintln(w, sep)

	var headerBuf strings.Builder
	fmt.Fprintf(&headerBuf, "| %-*s", groupWidth, "Group")
	for _, name := range runNames {
		fmt.Fprintf(&headerBuf, " | %-*s", colWidth, name)
	}
	headerBuf.WriteString(" |")
	fmt.Fprintln(w, headerBuf.String())
	fmt.Fprintln(w, sep)

	for _, g := range groups {
		var rowBuf strings.Builder
		fmt.Fprintf(&rowBuf, "| %-*s", groupWidth, g)
		for _, name := range runNames {
			s := runStats[name][g]
			cell := formatStats(s.score, s.total)
			fmt.Fprintf(&rowBuf, " | %-*s", colWidth, cell)
		}
		rowBuf.WriteString(" |")
		fmt.Fprintln(w, rowBuf.String())
	}

	fmt.Fprintln(w, sep)

	var totalBuf strings.Builder
	fmt.Fprintf(&totalBuf, "| %-*s", groupWidth, "TOTAL")
	for _, name := range runNames {
		s := runTotals[name]
		cell := formatStats(s.score, s.total)
		fmt.Fprintf(&totalBuf, " | %-*s", colWidth, cell)
	}
	totalBuf.WriteString(" |")
	fmt.Fprintln(w, totalBuf.String())
	fmt.Fprintln(w, sep)
}

func formatStats(score float64, total int) string {
	if total == 0 {
		return "N/A"
	}
	pct := score / float64(total) * 100
	return fmt.Sprintf("%5.1f%% (%.1f/%d)", pct, score, total)
}
