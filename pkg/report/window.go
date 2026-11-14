package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ParseWindow reads the report window: a number of days (7d), weeks (2w), or any
// Go duration (36h, 90m, 1h30m). The number may be fractional (0.5d). A window must
// be positive.
func ParseWindow(name, s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%s is empty: use a window such as 7d, 24h or 2w", name)
	}
	unit := time.Duration(0)
	switch s[len(s)-1] {
	case 'd':
		unit = 24 * time.Hour
	case 'w':
		unit = 7 * 24 * time.Hour
	}
	var d time.Duration
	if unit != 0 {
		n, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, fmt.Errorf("invalid %s %q: use a window such as 7d, 24h or 2w", name, s)
		}
		d = time.Duration(n * float64(unit))
	} else {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid %s %q: use a window such as 7d, 24h or 2w", name, s)
		}
		d = parsed
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid %s %q: the window must be positive", name, s)
	}
	return d, nil
}

// FormatWindow renders a window the way ParseWindow takes it.
func FormatWindow(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	return d.String()
}

// percentile is the nearest-rank percentile of xs (p in (0, 1]). It sorts a copy.
// It is only called with at least one sample.
func percentile(xs []float64, p float64) float64 {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// distribute summarises a stage's durations. No samples is no data, never 0s.
func distribute(seconds []float64) Distribution {
	if len(seconds) == 0 {
		return Distribution{NoData: true}
	}
	p50, p90 := percentile(seconds, 0.5), percentile(seconds, 0.9)
	return Distribution{N: len(seconds), P50Seconds: &p50, P90Seconds: &p90}
}

// ratio is num/den, or nil when there is nothing to divide by.
func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den)
	return &v
}

// since returns later - earlier in seconds when both are known and in order.
func secondsBetween(earlier, later time.Time, okA, okB bool) (float64, bool) {
	if !okA || !okB || later.Before(earlier) {
		return 0, false
	}
	return later.Sub(earlier).Seconds(), true
}
