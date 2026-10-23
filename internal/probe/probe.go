// Package probe reads GPU temperatures from inside a helper container.
//
// The extension backend runs in the Docker Desktop VM, which has no NVIDIA
// tools of its own. A container started with GPU access gets nvidia-smi and
// the driver libraries injected by the NVIDIA container toolkit, so the probe
// keeps one such container idle and runs nvidia-smi in it with docker exec.
package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
	"github.com/QuantumNovice/docker-gpu-governor/internal/sensor"
)

// ContainerName is the name of the helper container.
const ContainerName = "gpu-governor-probe"

// Engine is the part of the Docker API the probe uses.
type Engine interface {
	Inspect(ctx context.Context, id string) (docker.Details, error)
	Create(ctx context.Context, spec docker.CreateSpec) (string, error)
	Start(ctx context.Context, id string) error
	Remove(ctx context.Context, id string) error
	Exec(ctx context.Context, id string, argv []string) (string, error)
}

// Probe manages the helper container.
type Probe struct {
	engine Engine
	image  string
	retry  time.Duration
	now    func() time.Time

	mu        sync.Mutex
	id        string
	failedAt  time.Time
	lastError error
}

// New returns a probe that runs image as the helper. image must contain the
// gpu-governor binary at /gpu-governor.
func New(engine Engine, image string) *Probe {
	return &Probe{engine: engine, image: image, retry: time.Minute, now: time.Now}
}

// GPU reads GPU temperatures. When the helper cannot be started, usually
// because Docker has no GPU support on this machine, it returns
// sensor.ErrUnavailable and waits a minute before trying again.
func (p *Probe) GPU(ctx context.Context) ([]sensor.Reading, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.id == "" {
		if !p.failedAt.IsZero() && p.now().Sub(p.failedAt) < p.retry {
			return nil, p.lastError
		}
		if err := p.ensure(ctx); err != nil {
			p.failedAt = p.now()
			p.lastError = fmt.Errorf("%w: gpu probe: %v", sensor.ErrUnavailable, err)
			return nil, p.lastError
		}
		p.failedAt = time.Time{}
	}

	readings, err := sensor.GPU(ctx, func(ctx context.Context, argv []string) (string, error) {
		return p.engine.Exec(ctx, p.id, argv)
	})
	if err != nil {
		// The helper may have been removed or stopped. Look it up again on
		// the next call.
		p.id = ""
	}
	return readings, err
}

// ensure finds a running helper or starts a new one.
func (p *Probe) ensure(ctx context.Context) error {
	d, err := p.engine.Inspect(ctx, ContainerName)
	switch {
	case err == nil && d.State == "running" && d.Image == p.image:
		p.id = d.ID
		return nil
	case err == nil:
		// Stopped, paused or left over from another version.
		if err := p.engine.Remove(ctx, d.ID); err != nil {
			return err
		}
	case !errors.Is(err, docker.ErrNotFound):
		return err
	}

	id, err := p.engine.Create(ctx, docker.CreateSpec{
		Name:       ContainerName,
		Image:      p.image,
		Entrypoint: []string{"/gpu-governor"},
		Cmd:        []string{"idle"},
		Env:        []string{"NVIDIA_DRIVER_CAPABILITIES=utility"},
		Labels:     map[string]string{"gpu-governor.role": "probe"},
		GPUs:       true,
	})
	if err != nil {
		return err
	}
	if err := p.engine.Start(ctx, id); err != nil {
		_ = p.engine.Remove(ctx, id)
		return err
	}
	p.id = id
	return nil
}

// Close removes the helper container.
func (p *Probe) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.id == "" {
		return nil
	}
	err := p.engine.Remove(ctx, p.id)
	p.id = ""
	return err
}
