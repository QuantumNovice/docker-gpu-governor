// Package sensor reads CPU and GPU temperatures.
package sensor

import "errors"

// ErrUnavailable means the host exposes no usable sensor of the requested kind.
var ErrUnavailable = errors.New("sensor unavailable")

// Reading is a single temperature sample in degrees Celsius.
type Reading struct {
	Name  string  `json:"name"`
	TempC float64 `json:"tempC"`
}

// Max returns the hottest reading. ok is false when readings is empty.
func Max(readings []Reading) (hottest Reading, ok bool) {
	for i, r := range readings {
		if i == 0 || r.TempC > hottest.TempC {
			hottest = r
		}
	}
	return hottest, len(readings) > 0
}
