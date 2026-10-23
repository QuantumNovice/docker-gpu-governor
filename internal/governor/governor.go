package governor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
	"github.com/QuantumNovice/docker-gpu-governor/internal/sensor"
)

// Engine is the part of the Docker API the governor uses.
type Engine interface {
	ListContainers(ctx context.Context) ([]docker.Container, error)
	Inspect(ctx context.Context, id string) (docker.Details, error)
	Pause(ctx context.Context, id string) error
	Unpause(ctx context.Context, id string) error
}

// Sampler reads temperatures.
type Sampler interface {
	CPU(ctx context.Context) ([]sensor.Reading, error)
	GPU(ctx context.Context) ([]sensor.Reading, error)
}

// Config is the user-facing configuration.
type Config struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	IntervalSeconds int    `json:"intervalSeconds"`
	CPU             Limits `json:"cpu"`
	GPU             Limits `json:"gpu"`
}

// DefaultConfig returns limits that suit most desktop and laptop hardware.
// NVIDIA consumer cards start to throttle around 83 to 87 C and most CPUs
// around 95 to 100 C.
func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		Mode:            ModeGPU,
		IntervalSeconds: 5,
		GPU:             Limits{Enabled: true, PauseAtC: 83, ResumeAtC: 70, TripSamples: 3, ClearSamples: 6},
		CPU:             Limits{Enabled: true, PauseAtC: 92, ResumeAtC: 75, TripSamples: 3, ClearSamples: 6},
	}
}

// Validate rejects configurations that would make the governor misbehave.
func (c Config) Validate() error {
	if !ValidMode(c.Mode) {
		return fmt.Errorf("mode must be %q, %q or %q", ModeGPU, ModeLabeled, ModeAll)
	}
	if c.IntervalSeconds < 1 || c.IntervalSeconds > 300 {
		return errors.New("intervalSeconds must be between 1 and 300")
	}
	for name, l := range map[string]Limits{"cpu": c.CPU, "gpu": c.GPU} {
		if l.PauseAtC < 30 || l.PauseAtC > 120 {
			return fmt.Errorf("%s.pauseAtC must be between 30 and 120", name)
		}
		if l.ResumeAtC >= l.PauseAtC {
			return fmt.Errorf("%s.resumeAtC must be lower than %s.pauseAtC", name, name)
		}
		if l.TripSamples < 1 || l.TripSamples > 60 || l.ClearSamples < 1 || l.ClearSamples > 60 {
			return fmt.Errorf("%s sample counts must be between 1 and 60", name)
		}
	}
	return nil
}

// SensorStatus describes the latest reading of one sensor kind.
type SensorStatus struct {
	Available bool             `json:"available"`
	Readings  []sensor.Reading `json:"readings"`
	MaxC      float64          `json:"maxC"`
	Tripped   bool             `json:"tripped"`
	Error     string           `json:"error,omitempty"`
}

// PausedContainer records a container the governor paused.
type PausedContainer struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Since  time.Time `json:"since"`
	Reason string    `json:"reason"`
}

// ContainerStatus describes a running or paused container.
type ContainerStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Image            string `json:"image"`
	State            string `json:"state"`
	UsesGPU          bool   `json:"usesGPU"`
	Target           bool   `json:"target"`
	PausedByGovernor bool   `json:"pausedByGovernor"`
}

// Event is an entry in the activity log.
type Event struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Status is a snapshot of the governor.
type Status struct {
	Hot        bool              `json:"hot"`
	CPU        SensorStatus      `json:"cpu"`
	GPU        SensorStatus      `json:"gpu"`
	Containers []ContainerStatus `json:"containers"`
	Paused     []PausedContainer `json:"paused"`
	Events     []Event           `json:"events"`
	Config     Config            `json:"config"`
	UpdatedAt  time.Time         `json:"updatedAt"`
	Error      string            `json:"error,omitempty"`
}

const maxEvents = 200

// Governor pauses target containers while a sensor is over its limit and
// resumes them once every sensor is back in range. It only resumes
// containers it paused itself.
type Governor struct {
	engine  Engine
	sampler Sampler
	store   Store
	log     *slog.Logger
	now     func() time.Time

	// tickMu serialises ticks and guards the fields below it.
	tickMu   sync.Mutex
	cpuGate  Gate
	gpuGate  Gate
	paused   map[string]PausedContainer
	gpuCache map[string]bool
	// overrides holds containers the user resumed during the current hot
	// spell. They are left alone until every sensor recovers.
	overrides map[string]bool

	// mu guards cfg, status and events, which the API reads.
	mu     sync.Mutex
	cfg    Config
	status Status
	events []Event
}

// New returns a governor. Call Run to start it.
func New(engine Engine, sampler Sampler, cfg Config, log *slog.Logger) *Governor {
	if log == nil {
		log = slog.Default()
	}
	return &Governor{
		engine:    engine,
		sampler:   sampler,
		log:       log,
		now:       time.Now,
		cfg:       cfg,
		paused:    map[string]PausedContainer{},
		gpuCache:  map[string]bool{},
		overrides: map[string]bool{},
	}
}

// Restore loads saved settings and paused containers from store and saves
// every later change to it.
func (g *Governor) Restore(store Store) error {
	g.tickMu.Lock()
	defer g.tickMu.Unlock()
	g.store = store
	state, found, err := store.Load()
	if err != nil || !found {
		return err
	}
	if err := state.Config.Validate(); err == nil {
		g.mu.Lock()
		g.cfg = state.Config
		g.mu.Unlock()
	} else {
		g.event("warn", "ignoring saved settings: "+err.Error())
	}
	for _, p := range state.Paused {
		g.paused[p.ID] = p
	}
	if len(state.Paused) > 0 {
		g.event("info", fmt.Sprintf("restored %d paused container(s) from the last run", len(state.Paused)))
	}
	return nil
}

// save writes the current state. Callers hold tickMu.
func (g *Governor) save() {
	if g.store == nil {
		return
	}
	if err := g.store.Save(State{Config: g.Config(), Paused: g.pausedList()}); err != nil {
		g.event("error", "could not save state: "+err.Error())
	}
}

// Config returns the current configuration.
func (g *Governor) Config() Config {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cfg
}

// SetConfig replaces the configuration. It takes effect on the next tick.
func (g *Governor) SetConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	g.tickMu.Lock()
	defer g.tickMu.Unlock()
	g.mu.Lock()
	g.cfg = cfg
	g.mu.Unlock()
	g.event("info", "settings updated")
	g.save()
	return nil
}

// Status returns the latest snapshot.
func (g *Governor) Status() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.status
	s.Config = g.cfg
	s.Events = append([]Event(nil), g.events...)
	return s
}

// Run ticks until ctx is cancelled.
func (g *Governor) Run(ctx context.Context) {
	for {
		g.Tick(ctx)
		interval := time.Duration(g.Config().IntervalSeconds) * time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// Tick takes one sample and acts on it.
func (g *Governor) Tick(ctx context.Context) {
	g.tickMu.Lock()
	defer g.tickMu.Unlock()

	cfg := g.Config()
	cpu := g.observe(ctx, "CPU", g.sampler.CPU, cfg.CPU, &g.cpuGate)
	gpu := g.observe(ctx, "GPU", g.sampler.GPU, cfg.GPU, &g.gpuGate)
	hot := cfg.Enabled && (g.cpuGate.Tripped() || g.gpuGate.Tripped())

	status := Status{Hot: hot, CPU: cpu, GPU: gpu, UpdatedAt: g.now()}
	containers, err := g.engine.ListContainers(ctx)
	if err != nil {
		status.Error = "list containers: " + err.Error()
		g.publish(status)
		return
	}

	byID := make(map[string]docker.Container, len(containers))
	for _, c := range containers {
		byID[c.ID] = c
	}
	dirty := g.forgetGone(byID)
	if !hot {
		clear(g.overrides)
	}

	reason := hotReason(cpu, gpu)
	for _, c := range containers {
		usesGPU := g.usesGPU(ctx, c.ID)
		target := IsTarget(cfg.Mode, c, usesGPU)
		_, ours := g.paused[c.ID]
		if ours && c.State == "running" {
			delete(g.paused, c.ID)
			g.overrides[c.ID] = true
			ours, dirty = false, true
			g.event("info", c.Name+" was resumed by hand and will be left running until temperatures recover")
		}
		switch {
		case hot && target && c.State == "running" && !g.overrides[c.ID]:
			if err := g.engine.Pause(ctx, c.ID); err != nil {
				g.event("error", fmt.Sprintf("could not pause %s: %v", c.Name, err))
				break
			}
			c.State = "paused"
			g.paused[c.ID] = PausedContainer{ID: c.ID, Name: c.Name, Since: g.now(), Reason: reason}
			ours, dirty = true, true
			g.event("warn", fmt.Sprintf("paused %s (%s)", c.Name, reason))
		case !hot && ours && c.State == "paused":
			if err := g.engine.Unpause(ctx, c.ID); err != nil {
				g.event("error", fmt.Sprintf("could not resume %s: %v", c.Name, err))
				break
			}
			c.State = "running"
			delete(g.paused, c.ID)
			ours, dirty = false, true
			g.event("info", "resumed "+c.Name)
		}
		status.Containers = append(status.Containers, ContainerStatus{
			ID: c.ID, Name: c.Name, Image: c.Image, State: c.State,
			UsesGPU: usesGPU, Target: target, PausedByGovernor: ours,
		})
	}
	status.Paused = g.pausedList()
	g.publish(status)
	if dirty {
		g.save()
	}
}

// ResumeAll resumes every container the governor paused and keeps them
// running until temperatures recover.
func (g *Governor) ResumeAll(ctx context.Context) error {
	g.tickMu.Lock()
	defer g.tickMu.Unlock()
	var errs []error
	for id, p := range g.paused {
		if err := g.engine.Unpause(ctx, id); err != nil && !errors.Is(err, docker.ErrNotFound) {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
			continue
		}
		delete(g.paused, id)
		g.overrides[id] = true
		g.event("info", "resumed "+p.Name+" by request")
	}
	g.mu.Lock()
	g.status.Paused = g.pausedList()
	g.mu.Unlock()
	g.save()
	return errors.Join(errs...)
}

// Shutdown resumes every container the governor paused. It runs when the
// governor stops so that removing or disabling it never leaves workloads
// frozen with nothing left to thaw them. Containers it fails to resume stay
// in the saved state and are handled by the next run.
func (g *Governor) Shutdown(ctx context.Context) error {
	g.tickMu.Lock()
	defer g.tickMu.Unlock()
	var errs []error
	for id, p := range g.paused {
		if err := g.engine.Unpause(ctx, id); err != nil && !errors.Is(err, docker.ErrNotFound) {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
			continue
		}
		delete(g.paused, id)
		g.event("info", "resumed "+p.Name+" on shutdown")
	}
	g.save()
	return errors.Join(errs...)
}

func (g *Governor) observe(ctx context.Context, kind string, read func(context.Context) ([]sensor.Reading, error), l Limits, gate *Gate) SensorStatus {
	readings, err := read(ctx)
	hottest, ok := sensor.Max(readings)
	ok = ok && err == nil
	s := SensorStatus{Available: ok, Readings: readings, MaxC: hottest.TempC}
	// A bare ErrUnavailable just means the machine has no such sensor.
	// Anything wrapped around it explains why and is worth showing.
	if err != nil && err != sensor.ErrUnavailable {
		s.Error = err.Error()
	}
	if gate.Observe(l, hottest.TempC, ok) {
		switch {
		case !l.Enabled:
			g.event("info", kind+" limit turned off")
		case gate.Tripped():
			g.event("warn", fmt.Sprintf("%s at %.1f°C, over the %.0f°C limit", kind, hottest.TempC, l.PauseAtC))
		case ok:
			g.event("info", fmt.Sprintf("%s back to %.1f°C", kind, hottest.TempC))
		default:
			g.event("warn", kind+" sensor lost, releasing its limit")
		}
	}
	s.Tripped = gate.Tripped()
	return s
}

func (g *Governor) usesGPU(ctx context.Context, id string) bool {
	if v, ok := g.gpuCache[id]; ok {
		return v
	}
	d, err := g.engine.Inspect(ctx, id)
	if err != nil {
		return false
	}
	g.gpuCache[id] = d.UsesGPU
	return d.UsesGPU
}

// forgetGone drops state for containers that no longer exist.
func (g *Governor) forgetGone(existing map[string]docker.Container) (changed bool) {
	for id := range g.gpuCache {
		if _, ok := existing[id]; !ok {
			delete(g.gpuCache, id)
			delete(g.overrides, id)
		}
	}
	for id, p := range g.paused {
		if _, ok := existing[id]; !ok {
			delete(g.paused, id)
			changed = true
			g.event("info", p.Name+" was removed while paused")
		}
	}
	return changed
}

func (g *Governor) pausedList() []PausedContainer {
	out := make([]PausedContainer, 0, len(g.paused))
	for _, p := range g.paused {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

func (g *Governor) publish(s Status) {
	g.mu.Lock()
	g.status = s
	g.mu.Unlock()
}

func (g *Governor) event(level, msg string) {
	switch level {
	case "error":
		g.log.Error(msg)
	case "warn":
		g.log.Warn(msg)
	default:
		g.log.Info(msg)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.events = append(g.events, Event{Time: g.now(), Level: level, Message: msg})
	if len(g.events) > maxEvents {
		g.events = g.events[len(g.events)-maxEvents:]
	}
}

func hotReason(cpu, gpu SensorStatus) string {
	switch {
	case cpu.Tripped && gpu.Tripped:
		return fmt.Sprintf("CPU %.1f°C, GPU %.1f°C", cpu.MaxC, gpu.MaxC)
	case cpu.Tripped:
		return fmt.Sprintf("CPU %.1f°C", cpu.MaxC)
	default:
		return fmt.Sprintf("GPU %.1f°C", gpu.MaxC)
	}
}
