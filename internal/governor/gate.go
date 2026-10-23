// Package governor decides when to pause and resume containers based on
// temperature readings.
package governor

// Limits configures one temperature gate.
//
// The gate trips after TripSamples consecutive readings at or above PauseAtC
// and clears after ClearSamples consecutive readings at or below ResumeAtC.
// The gap between the two thresholds keeps containers from flapping between
// paused and running when the temperature hovers around a single limit.
type Limits struct {
	Enabled      bool    `json:"enabled"`
	PauseAtC     float64 `json:"pauseAtC"`
	ResumeAtC    float64 `json:"resumeAtC"`
	TripSamples  int     `json:"tripSamples"`
	ClearSamples int     `json:"clearSamples"`
}

// maxMissingSamples is how many failed reads in a row a tripped gate
// tolerates before it clears. Without it a sensor that disappears while hot
// would keep containers paused forever.
const maxMissingSamples = 12

// Gate tracks whether a single sensor is over its limit.
type Gate struct {
	tripped bool
	above   int
	below   int
	missing int
}

// Tripped reports whether the gate is currently holding containers paused.
func (g *Gate) Tripped() bool { return g.tripped }

// Reset clears the gate.
func (g *Gate) Reset() { *g = Gate{} }

// Observe feeds one reading into the gate. ok is false when the sensor could
// not be read. It returns true when the tripped state changed.
func (g *Gate) Observe(l Limits, tempC float64, ok bool) bool {
	before := g.tripped
	switch {
	case !l.Enabled:
		g.Reset()
	case !ok:
		g.above, g.below = 0, 0
		g.missing++
		if g.tripped && g.missing >= maxMissingSamples {
			g.tripped = false
		}
	case !g.tripped:
		g.missing = 0
		if tempC >= l.PauseAtC {
			g.above++
		} else {
			g.above = 0
		}
		if g.above >= max(1, l.TripSamples) {
			g.tripped, g.above = true, 0
		}
	default:
		g.missing = 0
		if tempC <= l.ResumeAtC {
			g.below++
		} else {
			g.below = 0
		}
		if g.below >= max(1, l.ClearSamples) {
			g.tripped, g.below = false, 0
		}
	}
	return g.tripped != before
}
