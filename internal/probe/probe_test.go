package probe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
	"github.com/QuantumNovice/docker-gpu-governor/internal/sensor"
)

type fakeEngine struct {
	existing  *docker.Details
	createErr error
	execOut   string
	execErr   error
	created   []docker.CreateSpec
	removed   []string
	execs     int
}

func (f *fakeEngine) Inspect(context.Context, string) (docker.Details, error) {
	if f.existing == nil {
		return docker.Details{}, docker.ErrNotFound
	}
	return *f.existing, nil
}

func (f *fakeEngine) Create(_ context.Context, spec docker.CreateSpec) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, spec)
	return "new", nil
}

func (f *fakeEngine) Start(context.Context, string) error { return nil }

func (f *fakeEngine) Remove(_ context.Context, id string) error {
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeEngine) Exec(context.Context, string, []string) (string, error) {
	f.execs++
	return f.execOut, f.execErr
}

func TestStartsHelperWithGPUAccess(t *testing.T) {
	e := &fakeEngine{execOut: "0, RTX, 64\n"}
	p := New(e, "img:1")
	got, err := p.GPU(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].TempC != 64 {
		t.Fatalf("got %+v", got)
	}
	if len(e.created) != 1 || !e.created[0].GPUs || e.created[0].Image != "img:1" || e.created[0].Name != ContainerName {
		t.Fatalf("created %+v", e.created)
	}

	if _, err := p.GPU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.created) != 1 {
		t.Fatal("the helper must be reused between samples")
	}
}

func TestReusesRunningHelper(t *testing.T) {
	e := &fakeEngine{existing: &docker.Details{Container: docker.Container{ID: "old", State: "running", Image: "img:1"}}, execOut: "0, RTX, 64\n"}
	if _, err := New(e, "img:1").GPU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.created) != 0 || len(e.removed) != 0 {
		t.Fatalf("created=%v removed=%v", e.created, e.removed)
	}
}

func TestReplacesHelperFromOtherVersion(t *testing.T) {
	e := &fakeEngine{existing: &docker.Details{Container: docker.Container{ID: "old", State: "running", Image: "img:0"}}, execOut: "0, RTX, 64\n"}
	if _, err := New(e, "img:1").GPU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.removed) != 1 || e.removed[0] != "old" || len(e.created) != 1 {
		t.Fatalf("created=%v removed=%v", e.created, e.removed)
	}
}

func TestNoGPUSupportBacksOff(t *testing.T) {
	e := &fakeEngine{createErr: errors.New(`could not select device driver "" with capabilities: [[gpu]]`)}
	p := New(e, "img:1")
	now := time.Unix(0, 0)
	p.now = func() time.Time { return now }

	_, err := p.GPU(context.Background())
	if !errors.Is(err, sensor.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	e.createErr = nil
	e.execOut = "0, RTX, 64\n"

	now = now.Add(30 * time.Second)
	if _, err := p.GPU(context.Background()); err == nil {
		t.Fatal("must not retry before the back-off expires")
	}
	now = now.Add(31 * time.Second)
	if _, err := p.GPU(context.Background()); err != nil {
		t.Fatalf("retry after back-off failed: %v", err)
	}
}

func TestExecFailureForgetsHelper(t *testing.T) {
	e := &fakeEngine{execErr: errors.New("container is not running")}
	p := New(e, "img:1")
	if _, err := p.GPU(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	e.execErr = nil
	e.execOut = "0, RTX, 64\n"
	if _, err := p.GPU(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.created) != 2 {
		t.Fatalf("expected the helper to be looked up again, created %d", len(e.created))
	}
}

func TestClose(t *testing.T) {
	e := &fakeEngine{execOut: "0, RTX, 64\n"}
	p := New(e, "img:1")
	_, _ = p.GPU(context.Background())
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.removed) != 1 || e.removed[0] != "new" {
		t.Fatalf("removed %v", e.removed)
	}
}
