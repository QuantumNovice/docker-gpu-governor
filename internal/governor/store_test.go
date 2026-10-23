package governor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "sub", "state.json")}
	if _, found, err := store.Load(); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	want := State{Config: DefaultConfig(), Paused: []PausedContainer{{ID: "a", Name: "a", Since: time.Unix(10, 0).UTC()}}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.Config != want.Config || len(got.Paused) != 1 || !got.Paused[0].Since.Equal(want.Paused[0].Since) {
		t.Fatalf("got %+v", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(store.Path), ".state-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestFileStoreCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (FileStore{Path: path}).Load(); err == nil {
		t.Fatal("expected an error for a corrupt file")
	}
}

func TestRestoreResumesContainersPausedByPreviousRun(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "state.json")}

	e := newFakeEngine()
	e.add("train", "running", true, nil)
	s := &fakeSampler{cpu: 50, gpu: 90}
	first := newTestGovernor(e, s)
	if err := first.Restore(store); err != nil {
		t.Fatal(err)
	}
	ticks(first, 1)
	if e.state("train") != "paused" {
		t.Fatal("setup: expected train to be paused")
	}

	// The backend restarts while the container is paused and the GPU has
	// cooled down in the meantime.
	s.gpu = 50
	second := newTestGovernor(e, s)
	if err := second.Restore(store); err != nil {
		t.Fatal(err)
	}
	if len(second.Status().Events) == 0 {
		t.Fatal("expected a restore event")
	}
	ticks(second, 1)
	if e.state("train") != "running" {
		t.Fatal("a restarted governor must resume what the previous run paused")
	}
	state, _, _ := store.Load()
	if len(state.Paused) != 0 {
		t.Fatalf("store still lists %v", state.Paused)
	}
}

func TestRestoreKeepsSavedSettings(t *testing.T) {
	store := FileStore{Path: filepath.Join(t.TempDir(), "state.json")}
	g := newTestGovernor(newFakeEngine(), &fakeSampler{})
	_ = g.Restore(store)
	cfg := g.Config()
	cfg.GPU.PauseAtC = 78
	if err := g.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	again := newTestGovernor(newFakeEngine(), &fakeSampler{})
	if err := again.Restore(store); err != nil {
		t.Fatal(err)
	}
	if again.Config().GPU.PauseAtC != 78 {
		t.Fatalf("config = %+v", again.Config())
	}
}
