package eval

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/timefmt-go"
)

// strftimeParse parses a string using a strftime format. Handles %f and %z
// with spec-compliant semantics, delegating all other directives to timefmt-go.
//
// %f semantics (parsing): optional — consumes leading '.' and 1-9 fractional
// digits if present, otherwise matches zero characters. Pads to nanoseconds.
//
// %z semantics (parsing): accepts 'Z', '+HH:MM', '-HH:MM', '+HHMM', '-HHMM'.
func strftimeParse(input, format string) (time.Time, error) {
	hasFrac := strings.Contains(format, "%f")
	hasZone := strings.Contains(format, "%z")

	cleanFmt := format
	cleanInput := input
	var nanos int

	if hasFrac {
		// Remove %f from format. Extract fractional seconds from input.
		cleanFmt = strings.Replace(cleanFmt, "%f", "", 1)
		cleanInput, nanos = extractFractionalSeconds(cleanInput, cleanFmt)
	}

	if hasZone {
		// Replace %z with timefmt-compatible format. timefmt supports %z
		// but only for +HHMM format. We need to normalize the input to
		// match what timefmt expects.
		cleanInput, cleanFmt = normalizeTimezone(cleanInput, cleanFmt)
	}

	t, err := timefmt.Parse(cleanInput, cleanFmt)
	if err != nil {
		return time.Time{}, err
	}

	if nanos > 0 {
		t = t.Add(time.Duration(nanos))
	}

	return t, nil
}

// strftimeFormat formats a timestamp using a strftime format. Handles %f and
// %z with spec-compliant semantics.
//
// %f semantics (formatting): emits shortest fractional seconds with leading
// dot, trailing zeros trimmed. Omitted entirely (including dot) when zero.
//
// %z semantics (formatting): 'Z' for UTC, '±HH:MM' for all other offsets.
func strftimeFormat(t time.Time, format string) string {
	hasFrac := strings.Contains(format, "%f")
	hasZone := strings.Contains(format, "%z")

	workFmt := format

	// Replace %f with a sentinel for post-processing.
	const fracSentinel = "\x00FRAC\x00"
	if hasFrac {
		workFmt = strings.Replace(workFmt, "%f", fracSentinel, 1)
	}

	// Replace %z with a sentinel for post-processing.
	const zoneSentinel = "\x00ZONE\x00"
	if hasZone {
		workFmt = strings.Replace(workFmt, "%z", zoneSentinel, 1)
	}

	// Format with timefmt (sentinels pass through as literals).
	result := timefmt.Format(t, workFmt)

	// Replace sentinels with spec-compliant values.
	if hasFrac {
		result = strings.Replace(result, fracSentinel, formatFractionalSeconds(t), 1)
	}
	if hasZone {
		result = strings.Replace(result, zoneSentinel, formatTimezone(t), 1)
	}

	return result
}

// extractFractionalSeconds finds and removes optional fractional seconds
// (a '.' followed by 1-9 digits) from the input string. Returns the
// cleaned input and the nanoseconds value.
//
// The position is determined by finding a '.' followed by digits that
// is NOT part of the format's literal text. We use the format (with %f
// already removed) to locate where the fractional seconds should appear.
func extractFractionalSeconds(input, formatWithoutF string) (string, int) {
	// Strategy: the fractional seconds appear as '.\d{1,9}' at a position
	// in the input that doesn't correspond to any format directive. Since
	// %f was between other directives (typically %S and %z or end), we
	// look for a '.' followed by digits that is not matched by the
	// cleaned format.
	//
	// Pragmatic approach: find all occurrences of \.\d{1,9} in the input
	// and try removing each one to see if the remaining string parses
	// with the cleaned format. Use the first one that works.
	//
	// Simpler approach for the common case: find a '.' followed by digits
	// that appears after the seconds portion. Since we can't easily
	// determine the seconds position from the format, we try all matches.
	re := regexp.MustCompile(`\.(\d{1,9})`)
	matches := re.FindAllStringIndex(input, -1)

	// Try removing each match (last to first to preserve indices).
	for i := len(matches) - 1; i >= 0; i-- {
		loc := matches[i]
		candidate := input[:loc[0]] + input[loc[1]:]
		if _, err := timefmt.Parse(candidate, formatWithoutF); err == nil {
			// This match is the fractional seconds.
			digits := input[loc[0]+1 : loc[1]] // skip the '.'
			nanos := parseFracDigits(digits)
			return candidate, nanos
		}
	}

	// No fractional seconds found — that's OK, %f is optional.
	return input, 0
}

// parseFracDigits parses fractional second digits (1-9) into nanoseconds.
func parseFracDigits(digits string) int {
	// Pad to 9 digits.
	for len(digits) < 9 {
		digits += "0"
	}
	if len(digits) > 9 {
		digits = digits[:9]
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// formatFractionalSeconds produces the spec-compliant %f output:
// shortest representation with leading dot, trailing zeros trimmed.
// Empty string when fractional seconds are zero.
func formatFractionalSeconds(t time.Time) string {
	ns := t.Nanosecond()
	if ns == 0 {
		return ""
	}
	// Format as 9-digit string, then trim trailing zeros.
	s := strconv.Itoa(ns)
	for len(s) < 9 {
		s = "0" + s
	}
	s = strings.TrimRight(s, "0")
	return "." + s
}

// normalizeTimezone adjusts the input string so that timezone offsets
// match what timefmt-go expects for %z parsing.
// timefmt-go's %z accepts: +HHMM, -HHMM, +HH:MM, -HH:MM, Z
func normalizeTimezone(input, format string) (string, string) {
	// timefmt-go handles %z reasonably well for parsing. The main issue
	// is that 'Z' needs to be handled. timefmt-go actually supports Z
	// in %z parsing, so we can pass through.
	return input, format
}

// formatTimezone produces the spec-compliant %z output:
// 'Z' for UTC, '±HH:MM' for all other offsets.
func formatTimezone(t time.Time) string {
	_, offset := t.Zone()
	if offset == 0 && t.Location() == time.UTC {
		return "Z"
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return sign + padInt(hours, 2) + ":" + padInt(minutes, 2)
}

func padInt(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
