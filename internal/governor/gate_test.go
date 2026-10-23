package governor

import "testing"

var testLimits = Limits{Enabled: true, PauseAtC: 85, ResumeAtC: 70, TripSamples: 2, ClearSamples: 3}

func feed(g *Gate, l Limits, temps ...float64) {
	for _, t := range temps {
		g.Observe(l, t, true)
	}
}

func TestGateNeedsConsecutiveHotSamples(t *testing.T) {
	var g Gate
	feed(&g, testLimits, 90, 60, 90)
	if g.Tripped() {
		t.Fatal("a single spike between cool samples must not trip the gate")
	}
	feed(&g, testLimits, 86)
	if !g.Tripped() {
		t.Fatal("two hot samples in a row must trip the gate")
	}
}

func TestGateHysteresis(t *testing.T) {
	var g Gate
	feed(&g, testLimits, 90, 90)
	feed(&g, testLimits, 80, 75, 72, 71)
	if !g.Tripped() {
		t.Fatal("gate must stay tripped between ResumeAtC and PauseAtC")
	}
	feed(&g, testLimits, 70, 69)
	if !g.Tripped() {
		t.Fatal("two cool samples are not enough")
	}
	feed(&g, testLimits, 75)
	feed(&g, testLimits, 68, 68)
	if !g.Tripped() {
		t.Fatal("a warm sample must restart the cool count")
	}
	feed(&g, testLimits, 68)
	if g.Tripped() {
		t.Fatal("three cool samples in a row must clear the gate")
	}
}

func TestGateReportsChanges(t *testing.T) {
	var g Gate
	if g.Observe(testLimits, 90, true) {
		t.Fatal("no change expected after the first hot sample")
	}
	if !g.Observe(testLimits, 90, true) {
		t.Fatal("change expected when the gate trips")
	}
}

func TestGateMissingReadings(t *testing.T) {
	var g Gate
	feed(&g, testLimits, 90, 90)
	for i := 0; i < maxMissingSamples-1; i++ {
		g.Observe(testLimits, 0, false)
	}
	if !g.Tripped() {
		t.Fatal("a short sensor outage must not release paused containers")
	}
	g.Observe(testLimits, 0, false)
	if g.Tripped() {
		t.Fatal("a long sensor outage must release the gate")
	}
}

func TestGateMissingReadingBreaksStreak(t *testing.T) {
	var g Gate
	g.Observe(testLimits, 90, true)
	g.Observe(testLimits, 0, false)
	g.Observe(testLimits, 90, true)
	if g.Tripped() {
		t.Fatal("hot samples separated by a failed read are not consecutive")
	}
}

func TestGateDisabled(t *testing.T) {
	var g Gate
	feed(&g, testLimits, 90, 90)
	off := testLimits
	off.Enabled = false
	if !g.Observe(off, 99, true) || g.Tripped() {
		t.Fatal("disabling the limit must clear the gate")
	}
}
