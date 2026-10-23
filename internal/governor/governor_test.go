package governor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
	"github.com/QuantumNovice/docker-gpu-governor/internal/sensor"
)

type fakeEngine struct {
	mu         sync.Mutex
	containers map[string]*docker.Container
	gpu        map[string]bool
	calls      []string
	pauseErr   error
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{containers: map[string]*docker.Container{}, gpu: map[string]bool{}}
}

func (f *fakeEngine) add(id, state string, gpu bool, labels map[string]string) {
	f.containers[id] = &docker.Container{ID: id, Name: id, State: state, Labels: labels}
	f.gpu[id] = gpu
}

func (f *fakeEngine) state(id string) string { return f.containers[id].State }

func (f *fakeEngine) ListContainers(context.Context) ([]docker.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.Container
	for _, c := range f.containers {
		out = append(out, *c)
	}
	return out, nil
}

func (f *fakeEngine) Inspect(_ context.Context, id string) (docker.Details, error) {
	c, ok := f.containers[id]
	if !ok {
		return docker.Details{}, docker.ErrNotFound
	}
	return docker.Details{Container: *c, UsesGPU: f.gpu[id]}, nil
}

func (f *fakeEngine) Pause(_ context.Context, id string) error {
	f.calls = append(f.calls, "pause "+id)
	if f.pauseErr != nil {
		return f.pauseErr
	}
	f.containers[id].State = "paused"
	return nil
}

func (f *fakeEngine) Unpause(_ context.Context, id string) error {
	f.calls = append(f.calls, "unpause "+id)
	f.containers[id].State = "running"
	return nil
}

type fakeSampler struct {
	cpu, gpu       float64
	cpuErr, gpuErr error
}

func (s *fakeSampler) CPU(context.Context) ([]sensor.Reading, error) {
	if s.cpuErr != nil {
		return nil, s.cpuErr
	}
	return []sensor.Reading{{Name: "cpu", TempC: s.cpu}}, nil
}

func (s *fakeSampler) GPU(context.Context) ([]sensor.Reading, error) {
	if s.gpuErr != nil {
		return nil, s.gpuErr
	}
	return []sensor.Reading{{Name: "gpu", TempC: s.gpu}}, nil
}

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.GPU = Limits{Enabled: true, PauseAtC: 85, ResumeAtC: 70, TripSamples: 1, ClearSamples: 2}
	cfg.CPU = Limits{Enabled: true, PauseAtC: 90, ResumeAtC: 75, TripSamples: 1, ClearSamples: 2}
	return cfg
}

func newTestGovernor(e Engine, s Sampler) *Governor {
	return New(e, s, testConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func ticks(g *Governor, n int) {
	for i := 0; i < n; i++ {
		g.Tick(context.Background())
	}
}

func TestPausesGPUContainersWhenHotAndResumesWhenCool(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	e.add("web", "running", false, nil)
	s := &fakeSampler{cpu: 50, gpu: 60}
	g := newTestGovernor(e, s)

	ticks(g, 1)
	if e.state("train") != "running" {
		t.Fatal("nothing should be paused while cool")
	}

	s.gpu = 88
	ticks(g, 1)
	if e.state("train") != "paused" || e.state("web") != "running" {
		t.Fatalf("train=%s web=%s", e.state("train"), e.state("web"))
	}
	if st := g.Status(); !st.Hot || len(st.Paused) != 1 || st.Paused[0].Reason != "GPU 88.0°C" {
		t.Fatalf("status = %+v", st)
	}

	s.gpu = 72
	ticks(g, 3)
	if e.state("train") != "paused" {
		t.Fatal("must stay paused until the temperature drops below resumeAtC")
	}

	s.gpu = 65
	ticks(g, 1)
	if e.state("train") != "paused" {
		t.Fatal("one cool sample is not enough with ClearSamples=2")
	}
	ticks(g, 1)
	if e.state("train") != "running" {
		t.Fatal("expected train to be resumed")
	}
	if len(g.Status().Paused) != 0 {
		t.Fatal("paused list should be empty after resume")
	}
}

func TestCPUHeatAlsoPauses(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	s := &fakeSampler{cpu: 95, gpu: 40}
	g := newTestGovernor(e, s)
	ticks(g, 1)
	if e.state("train") != "paused" {
		t.Fatal("cpu over its limit must pause targets")
	}
}

func TestNeverResumesContainersPausedByUser(t *testing.T) {
	e := newFakeEngine()
	e.add("mine", "paused", true, nil)
	s := &fakeSampler{cpu: 50, gpu: 90}
	g := newTestGovernor(e, s)
	ticks(g, 1)
	s.gpu = 50
	ticks(g, 3)
	if e.state("mine") != "paused" {
		t.Fatal("a container paused by the user must stay paused")
	}
	if len(e.calls) != 0 {
		t.Fatalf("unexpected calls %v", e.calls)
	}
}

func TestDisablingResumesEverything(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	s := &fakeSampler{cpu: 50, gpu: 90}
	g := newTestGovernor(e, s)
	ticks(g, 1)

	cfg := g.Config()
	cfg.Enabled = false
	if err := g.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	ticks(g, 1)
	if e.state("train") != "running" {
		t.Fatal("turning the governor off must resume what it paused")
	}
}

func TestPauseFailureIsRetried(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	e.pauseErr = errors.New("boom")
	s := &fakeSampler{cpu: 50, gpu: 90}
	g := newTestGovernor(e, s)
	ticks(g, 1)
	if len(g.Status().Paused) != 0 {
		t.Fatal("a failed pause must not be recorded")
	}
	e.pauseErr = nil
	ticks(g, 1)
	if e.state("train") != "paused" {
		t.Fatal("pause should be retried on the next tick")
	}
}

func TestRemovedContainerIsForgotten(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	s := &fakeSampler{cpu: 50, gpu: 90}
	g := newTestGovernor(e, s)
	ticks(g, 1)
	delete(e.containers, "train")
	ticks(g, 1)
	if len(g.Status().Paused) != 0 {
		t.Fatal("removed container should be dropped from the paused list")
	}
}

func TestMissingSensorDoesNotPause(t *testing.T) {
	e := newFakeEngine()
	e.add("train", "running", true, nil)
	s := &fakeSampler{cpuErr: sensor.ErrUnavailable, gpuErr: sensor.ErrUnavailable}
	g := newTestGovernor(e, s)
	ticks(g, 5)
	if e.state("train") != "running" {
		t.Fatal("no readings must never pause anything")
	}
	st := g.Status()
	if st.CPU.Available || st.GPU.Available || st.CPU.Error != "" {
		t.Fatalf("status = %+v", st)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	bad := DefaultConfig()
	bad.GPU.ResumeAtC = bad.GPU.PauseAtC
	if bad.Validate() == nil {
		t.Fatal("resumeAtC equal to pauseAtC must be rejected")
	}
	bad = DefaultConfig()
	bad.Mode = "some"
	if bad.Validate() == nil {
		t.Fatal("unknown mode must be rejected")
	}
	bad = DefaultConfig()
	bad.IntervalSeconds = 0
	if bad.Validate() == nil {
		t.Fatal("zero interval must be rejected")
	}
}
