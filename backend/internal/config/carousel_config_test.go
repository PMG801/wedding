package config

import (
	"strings"
	"testing"
	"time"
)

// loadWithEnvironment loads a valid base configuration plus extra variables.
func loadWithEnvironment(t *testing.T, extra map[string]string) (Config, error) {
	t.Helper()
	values := validEnvironment()
	for key, value := range extra {
		values[key] = value
	}
	return loadFrom(values)
}

func TestCarouselDefaultsWhenNoPeriodsAreConfigured(t *testing.T) {
	cfg, err := loadWithEnvironment(t, nil)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if len(cfg.Periods) != 0 {
		t.Fatalf("Periods = %+v; want none (feature off)", cfg.Periods)
	}
	if cfg.CarouselSize != 10 || cfg.CarouselRotation != 30*time.Minute {
		t.Fatalf("carousel defaults = %d, %v; want 10, 30m", cfg.CarouselSize, cfg.CarouselRotation)
	}
}

func TestCarouselParsesConsecutivePeriodsSizeAndRotation(t *testing.T) {
	cfg, err := loadWithEnvironment(t, map[string]string{
		"BODA_PERIOD_1_START":    "2026-10-18T18:00",
		"BODA_PERIOD_1_END":      "2026-10-18T20:00",
		"BODA_PERIOD_2_START":    "2026-10-18T20:00",
		"BODA_PERIOD_2_END":      "2026-10-18T23:59",
		"BODA_CAROUSEL_SIZE":     "50",
		"BODA_CAROUSEL_ROTATION": "1m",
	})
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	want := []Period{
		{ID: 1, Start: "2026-10-18T18:00", End: "2026-10-18T20:00"},
		{ID: 2, Start: "2026-10-18T20:00", End: "2026-10-18T23:59"},
	}
	if len(cfg.Periods) != len(want) || cfg.Periods[0] != want[0] || cfg.Periods[1] != want[1] {
		t.Fatalf("Periods = %+v; want %+v", cfg.Periods, want)
	}
	if cfg.CarouselSize != 50 || cfg.CarouselRotation != time.Minute {
		t.Fatalf("carousel = %d, %v; want 50, 1m", cfg.CarouselSize, cfg.CarouselRotation)
	}
}

func TestCarouselRejectsInvalidPeriodsNamingTheVariableNotTheValue(t *testing.T) {
	const secretLikeValue = "2026-10-18 18:00"
	for _, test := range []struct {
		name      string
		extra     map[string]string
		wantInErr string
	}{
		{"lone start", map[string]string{"BODA_PERIOD_1_START": "2026-10-18T18:00"}, "BODA_PERIOD_1_END"},
		{"lone end", map[string]string{"BODA_PERIOD_1_END": "2026-10-18T20:00"}, "BODA_PERIOD_1_START"},
		{"bad start value", map[string]string{"BODA_PERIOD_1_START": secretLikeValue, "BODA_PERIOD_1_END": "2026-10-18T20:00"}, "BODA_PERIOD_1_START"},
		{"bad end value", map[string]string{"BODA_PERIOD_1_START": "2026-10-18T18:00", "BODA_PERIOD_1_END": "2026-02-30T20:00"}, "BODA_PERIOD_1_END"},
		{"start equal to end", map[string]string{"BODA_PERIOD_1_START": "2026-10-18T18:00", "BODA_PERIOD_1_END": "2026-10-18T18:00"}, "BODA_PERIOD_1_END"},
		{"start after end", map[string]string{"BODA_PERIOD_1_START": "2026-10-18T19:00", "BODA_PERIOD_1_END": "2026-10-18T18:00"}, "BODA_PERIOD_1_END"},
		{"gap in numbering", map[string]string{
			"BODA_PERIOD_1_START": "2026-10-18T18:00", "BODA_PERIOD_1_END": "2026-10-18T19:00",
			"BODA_PERIOD_3_START": "2026-10-18T20:00", "BODA_PERIOD_3_END": "2026-10-18T21:00",
		}, "BODA_PERIOD_2_START"},
		{"gap beyond any small probe window", map[string]string{
			"BODA_PERIOD_1_START": "2026-10-18T18:00", "BODA_PERIOD_1_END": "2026-10-18T19:00",
			"BODA_PERIOD_150_START": "2026-10-18T20:00", "BODA_PERIOD_150_END": "2026-10-18T21:00",
		}, "BODA_PERIOD_2_START"},
		{"period not starting at one", map[string]string{"BODA_PERIOD_2_START": "2026-10-18T18:00", "BODA_PERIOD_2_END": "2026-10-18T19:00"}, "BODA_PERIOD_1_START"},
		{"non-canonical index", map[string]string{"BODA_PERIOD_01_START": "2026-10-18T18:00", "BODA_PERIOD_01_END": "2026-10-18T19:00"}, "BODA_PERIOD_01_"},
		{"unknown suffix", map[string]string{"BODA_PERIOD_1_MIDDLE": "2026-10-18T18:00"}, "BODA_PERIOD_1_MIDDLE"},
		{"size zero", map[string]string{"BODA_CAROUSEL_SIZE": "0"}, "BODA_CAROUSEL_SIZE"},
		{"size above fifty", map[string]string{"BODA_CAROUSEL_SIZE": "51"}, "BODA_CAROUSEL_SIZE"},
		{"size not a number", map[string]string{"BODA_CAROUSEL_SIZE": "many"}, "BODA_CAROUSEL_SIZE"},
		{"rotation below one minute", map[string]string{"BODA_CAROUSEL_ROTATION": "45s"}, "BODA_CAROUSEL_ROTATION"},
		{"rotation not a duration", map[string]string{"BODA_CAROUSEL_ROTATION": "half-hour"}, "BODA_CAROUSEL_ROTATION"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadWithEnvironment(t, test.extra)
			if err == nil {
				t.Fatal("LoadFrom() error = nil; want rejection")
			}
			if !strings.Contains(err.Error(), test.wantInErr) {
				t.Errorf("error %q does not name %s", err, test.wantInErr)
			}
			if strings.Contains(err.Error(), secretLikeValue) || strings.Contains(err.Error(), "2026-02-30") {
				t.Errorf("error %q leaks a configured period value", err)
			}
		})
	}
}
