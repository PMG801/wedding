package upload

import "testing"

func TestNormalizeCapturedAtAcceptsOnlyRealLocalTimesBetween2000And2100(t *testing.T) {
	for _, value := range []string{"2026-10-18T18:00:00", "2000-01-01T00:00:00", "2100-12-31T23:59:59", "2024-02-29T12:00:00"} {
		got, ok := NormalizeCapturedAt(value)
		if !ok || got != value {
			t.Errorf("NormalizeCapturedAt(%q) = %q, %v; want the same value accepted", value, got, ok)
		}
	}
	for _, value := range []string{
		"",
		" 2026-10-18T18:00:00",
		"2026-10-18 18:00:00",
		"2026-10-18T18:00",
		"2026-10-18T18:00:00Z",
		"2026-10-18T18:00:00.000",
		"1999-12-31T23:59:59",
		"2101-01-01T00:00:00",
		"2026-02-30T10:00:00",
		"2023-02-29T10:00:00",
		"2026-13-01T10:00:00",
		"2026-10-18T24:00:00",
		"2026-10-18T18:60:00",
		"2026-10-18T18:00:60",
	} {
		if got, ok := NormalizeCapturedAt(value); ok {
			t.Errorf("NormalizeCapturedAt(%q) = %q; want rejection", value, got)
		}
	}
}
