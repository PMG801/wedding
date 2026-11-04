package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Period is one carousel time window from BODA_PERIOD_<n>_START/END (ADR-0015).
// Start and End hold the configured YYYY-MM-DDTHH:MM values in event local time;
// the window is half-open: [Start, End).
type Period struct {
	ID    int
	Start string
	End   string
}

const (
	periodPrefix        = "BODA_PERIOD_"
	periodTimeLayout    = "2006-01-02T15:04"
	defaultCarouselN    = 10
	maxCarouselN        = 50
	defaultRotation     = 30 * time.Minute
	minimumRotation     = time.Minute
	carouselSizeKey     = "BODA_CAROUSEL_SIZE"
	carouselRotationKey = "BODA_CAROUSEL_ROTATION"
)

// loadPeriods builds the carousel periods from the environment variable names.
// Periods must be numbered 1..N with no gaps, each with both bounds, in the
// canonical time shape, and START < END. Errors name the variable, never its value.
func loadPeriods(lookup func(string) (string, bool), names []string) ([]Period, error) {
	type bounds struct {
		start, end       string
		hasStart, hasEnd bool
	}
	byIndex := map[int]*bounds{}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for _, name := range sorted {
		if !strings.HasPrefix(name, periodPrefix) {
			continue
		}
		if value, ok := lookup(name); !ok || value == "" {
			continue
		}
		index, suffix, found := strings.Cut(strings.TrimPrefix(name, periodPrefix), "_")
		number, err := strconv.Atoi(index)
		if !found || err != nil || number < 1 || strconv.Itoa(number) != index || (suffix != "START" && suffix != "END") {
			return nil, fmt.Errorf("%s is not a valid period variable; use %s<n>_START or %s<n>_END with n from 1", name, periodPrefix, periodPrefix)
		}
		entry := byIndex[number]
		if entry == nil {
			entry = &bounds{}
			byIndex[number] = entry
		}
		value, _ := lookup(name)
		if suffix == "START" {
			entry.start, entry.hasStart = value, true
		} else {
			entry.end, entry.hasEnd = value, true
		}
	}

	numbers := make([]int, 0, len(byIndex))
	for number := range byIndex {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)

	periods := make([]Period, 0, len(numbers))
	for position, number := range numbers {
		if number != position+1 {
			return nil, fmt.Errorf("%s is required: period numbers must be consecutive from 1", periodVariable(position+1, "START"))
		}
		entry := byIndex[number]
		if !entry.hasStart {
			return nil, fmt.Errorf("%s is required when %s is set", periodVariable(number, "START"), periodVariable(number, "END"))
		}
		if !entry.hasEnd {
			return nil, fmt.Errorf("%s is required when %s is set", periodVariable(number, "END"), periodVariable(number, "START"))
		}
		start, ok := parsePeriodTime(entry.start)
		if !ok {
			return nil, fmt.Errorf("%s must be YYYY-MM-DDTHH:MM", periodVariable(number, "START"))
		}
		end, ok := parsePeriodTime(entry.end)
		if !ok {
			return nil, fmt.Errorf("%s must be YYYY-MM-DDTHH:MM", periodVariable(number, "END"))
		}
		if !start.Before(end) {
			return nil, fmt.Errorf("%s must be later than %s", periodVariable(number, "END"), periodVariable(number, "START"))
		}
		periods = append(periods, Period{ID: number, Start: entry.start, End: entry.end})
	}
	return periods, nil
}

func periodVariable(number int, suffix string) string {
	return periodPrefix + strconv.Itoa(number) + "_" + suffix
}

// parsePeriodTime accepts only the canonical YYYY-MM-DDTHH:MM shape with a real date.
func parsePeriodTime(value string) (time.Time, bool) {
	if len(value) != len(periodTimeLayout) {
		return time.Time{}, false
	}
	parsed, err := time.Parse(periodTimeLayout, value)
	if err != nil || parsed.Format(periodTimeLayout) != value {
		return time.Time{}, false
	}
	return parsed, true
}

func carouselSize(lookup func(string) (string, bool)) (int, error) {
	value, ok := lookup(carouselSizeKey)
	if !ok || value == "" {
		return defaultCarouselN, nil
	}
	size, err := strconv.Atoi(value)
	if err != nil || size < 1 || size > maxCarouselN {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", carouselSizeKey, maxCarouselN)
	}
	return size, nil
}

func carouselRotation(lookup func(string) (string, bool)) (time.Duration, error) {
	value, ok := lookup(carouselRotationKey)
	if !ok || value == "" {
		return defaultRotation, nil
	}
	rotation, err := time.ParseDuration(value)
	if err != nil || rotation < minimumRotation {
		return 0, fmt.Errorf("%s must be a Go duration of at least %s", carouselRotationKey, minimumRotation)
	}
	return rotation, nil
}
