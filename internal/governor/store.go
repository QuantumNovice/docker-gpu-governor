package governor

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// State is what the governor keeps across restarts: the settings and the
// containers it paused, so they can still be resumed after the backend is
// restarted or upgraded while they are paused.
type State struct {
	Config Config            `json:"config"`
	Paused []PausedContainer `json:"paused"`
}

// Store loads and saves State.
type Store interface {
	Load() (State, bool, error)
	Save(State) error
}

// FileStore keeps State in a JSON file.
type FileStore struct {
	Path string
}

// Load reads the state file. found is false when the file does not exist.
func (f FileStore) Load() (s State, found bool, err error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, false, err
	}
	return s, true, nil
}

// Save writes the state file atomically so a crash mid-write cannot leave
// a truncated file behind.
func (f FileStore) Save(s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}
