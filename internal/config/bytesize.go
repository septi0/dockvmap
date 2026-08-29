package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

var byteUnits = []struct {
	suffix string
	factor float64
}{
	{"TB", 1 << 40}, {"T", 1 << 40},
	{"GB", 1 << 30}, {"G", 1 << 30},
	{"MB", 1 << 20}, {"M", 1 << 20},
	{"KB", 1 << 10}, {"K", 1 << 10},
	{"B", 1},
}

func parseBytes(raw string) (int64, error) {
	s := strings.TrimSpace(raw)

	if s == "" {
		return 0, nil
	}

	upper := strings.ToUpper(s)

	for _, unit := range byteUnits {
		if !strings.HasSuffix(upper, unit.suffix) {
			continue
		}

		value, err := strconv.ParseFloat(strings.TrimSpace(upper[:len(upper)-len(unit.suffix)]), 64)

		if err != nil {
			return 0, fmt.Errorf("invalid byte size %q", raw)
		}

		if value < 0 {
			return 0, fmt.Errorf("invalid byte size %q: must not be negative", raw)
		}

		scaled := value * unit.factor

		if scaled > math.MaxInt64 {
			return 0, fmt.Errorf("invalid byte size %q: too large", raw)
		}

		return int64(scaled), nil
	}

	value, err := strconv.ParseInt(s, 10, 64)

	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid byte size %q", raw)
	}

	return value, nil
}
