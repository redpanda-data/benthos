package eval

import (
	"testing"
	"time"
)

func TestStrftimeParse_DefaultFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{"no frac", "2024-01-15T10:30:00Z", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"millis", "2024-01-15T10:30:00.123Z", time.Date(2024, 1, 15, 10, 30, 0, 123000000, time.UTC)},
		{"nanos", "2024-01-15T10:30:00.123456789Z", time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC)},
		{"positive offset", "2024-01-15T10:30:00+05:30", time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", 5*3600+30*60))},
		{"negative offset", "2024-01-15T10:30:00-08:00", time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", -8*3600))},
		{"frac with offset", "2024-01-15T10:30:00.5+01:00", time.Date(2024, 1, 15, 10, 30, 0, 500000000, time.FixedZone("", 3600))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := strftimeParse(tt.input, defaultTimestampFormat)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			if got.Nanosecond() != tt.want.Nanosecond() {
				t.Fatalf("nanos: expected %d, got %d", tt.want.Nanosecond(), got.Nanosecond())
			}
		})
	}
}

func TestStrftimeParse_CustomFormat(t *testing.T) {
	got, err := strftimeParse("2024-01-15", "%Y-%m-%d")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 15 {
		t.Fatalf("expected 2024-01-15, got %v", got)
	}
}

func TestStrftimeFormat_DefaultFormat(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"no frac", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), "2024-01-15T10:30:00Z"},
		{"millis", time.Date(2024, 1, 15, 10, 30, 0, 123000000, time.UTC), "2024-01-15T10:30:00.123Z"},
		{"nanos", time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC), "2024-01-15T10:30:00.123456789Z"},
		{"trailing zeros trimmed", time.Date(2024, 1, 15, 10, 30, 0, 500000000, time.UTC), "2024-01-15T10:30:00.5Z"},
		{"offset", time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("EST", -5*3600)), "2024-01-15T10:30:00-05:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strftimeFormat(tt.t, defaultTimestampFormat)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestStrftimeFormat_CustomFormat(t *testing.T) {
	ts := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	got := strftimeFormat(ts, "%Y-%m-%d")
	if got != "2024-01-15" {
		t.Fatalf("expected %q, got %q", "2024-01-15", got)
	}
}

func TestStrftimeRoundTrip(t *testing.T) {
	original := time.Date(2024, 3, 1, 12, 30, 45, 123456789, time.UTC)
	formatted := strftimeFormat(original, defaultTimestampFormat)
	parsed, err := strftimeParse(formatted, defaultTimestampFormat)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}
	if !parsed.Equal(original) {
		t.Fatalf("round-trip failed: %v != %v", original, parsed)
	}
	if parsed.Nanosecond() != original.Nanosecond() {
		t.Fatalf("nanos lost: %d != %d", original.Nanosecond(), parsed.Nanosecond())
	}
}
