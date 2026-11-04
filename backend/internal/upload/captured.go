package upload

import "time"

// capturedAtLayout is the wall-clock shape of a capture time (ADR-0015): local
// time, no zone, YYYY-MM-DDTHH:MM:SS.
const capturedAtLayout = "2006-01-02T15:04:05"

// NormalizeCapturedAt reports whether value is a capture time the server stores:
// exactly the YYYY-MM-DDTHH:MM:SS shape, a real calendar date and time, and a
// year from 2000 to 2100. Anything else is ignored, never an upload error.
func NormalizeCapturedAt(value string) (string, bool) {
	if len(value) != len(capturedAtLayout) {
		return "", false
	}
	parsed, err := time.Parse(capturedAtLayout, value)
	if err != nil || parsed.Format(capturedAtLayout) != value {
		return "", false
	}
	if year := parsed.Year(); year < 2000 || year > 2100 {
		return "", false
	}
	return value, true
}
