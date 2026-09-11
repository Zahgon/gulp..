package cli

import (
	"math"
	"strconv"
	"strings"
	"time"
)

type durationUnit struct {
	name string
	size int64
}

// durationUnits are ordered coarsest first, so the formatter reports the
// largest unit the duration fills.
var durationUnits = []durationUnit{
	{"h", int64(time.Hour)},
	{"min", int64(time.Minute)},
	{"s", int64(time.Second)},
	{"ms", int64(time.Millisecond)},
	{"μs", int64(time.Microsecond)},
}

// FormatDuration renders d the way gulp reports task timings, for example
// "1.32 s", "150 ms" or "12 μs".
//
// This is a port of gulp-cli's format-hrtime rather than a use of Go's
// time.Duration.String, whose output ("1.32s", "1m0s") differs in both
// spacing and unit selection. Three significant digits are kept below ten
// units and the value is rounded to a whole number above that, with trailing
// zeros trimmed so a round second prints as "1 s" and not "1.00 s".
func FormatDuration(d time.Duration) string {
	nano := int64(d)
	if nano < 0 {
		nano = -nano
	}
	for _, unit := range durationUnits {
		if nano < unit.size {
			continue
		}
		if nano >= unit.size*10 {
			return strconv.FormatInt(roundDiv(nano, unit.size), 10) + " " + unit.name
		}
		return trimSignificant(roundDiv(nano*100, unit.size)) + " " + unit.name
	}
	if nano > 0 {
		return strconv.FormatInt(nano, 10) + " ns"
	}
	return ""
}

// trimSignificant turns the hundredths-scaled integer back into a decimal,
// dropping the fraction entirely when it is zero and dropping only the
// trailing digit when the hundredths place is zero.
func trimSignificant(scaled int64) string {
	s := strconv.FormatInt(scaled, 10)
	if len(s) < 3 {
		return s
	}
	whole, frac := s[:len(s)-2], s[len(s)-2:]
	switch {
	case frac == "00":
		return whole
	case strings.HasSuffix(frac, "0"):
		return whole + "." + frac[:1]
	default:
		return whole + "." + frac
	}
}

func roundDiv(value, unit int64) int64 {
	return int64(math.Round(float64(value) / float64(unit)))
}
