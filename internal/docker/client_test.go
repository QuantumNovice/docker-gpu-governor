package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func frame(stream byte, payload string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return append(header, payload...)
}

func TestDemux(t *testing.T) {
	var in bytes.Buffer
	in.Write(frame(1, "hello "))
	in.Write(frame(2, "oops"))
	in.Write(frame(1, "world"))
	var out, errOut bytes.Buffer
	if err := demux(&in, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello world" || errOut.String() != "oops" {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestDemuxTruncated(t *testing.T) {
	in := bytes.NewReader(frame(1, "hello")[:10])
	if err := demux(in, io.Discard, io.Discard); err == nil {
		t.Fatal("expected an error for a truncated frame")
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewWithHTTP(srv.URL, srv.Client())
}

func TestListContainers(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.41/containers/json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Query().Get("filters"), "paused") {
			t.Errorf("filters = %s", r.URL.Query().Get("filters"))
		}
		_, _ = io.WriteString(w, `[{"Id":"abc123","Names":["/trainer"],"Image":"pytorch","State":"running","Labels":{"a":"b"}}]`)
	})
	got, err := c.ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "trainer" || got[0].Labels["a"] != "b" {
		t.Fatalf("got %+v", got)
	}
}

func TestInspectDetectsGPU(t *testing.T) {
	cases := map[string]bool{
		`{"Id":"x","HostConfig":{"DeviceRequests":[{"Driver":"","Capabilities":[["gpu"]]}]}}`: true,
		`{"Id":"x","HostConfig":{"DeviceRequests":[{"Driver":"nvidia"}]}}`:                    true,
		`{"Id":"x","HostConfig":{"Runtime":"nvidia"}}`:                                        true,
		`{"Id":"x","HostConfig":{"Runtime":"runc","DeviceRequests":null}}`:                    false,
	}
	for body, want := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
		d, err := c.Inspect(context.Background(), "x")
		if err != nil {
			t.Fatal(err)
		}
		if d.UsesGPU != want {
			t.Errorf("%s: UsesGPU = %v, want %v", body, d.UsesGPU, want)
		}
	}
}

func TestPauseNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1.41/containers/gone/pause" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"No such container: gone"}`)
	})
	err := c.Pause(context.Background(), "gone")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "No such container") {
		t.Fatalf("err = %v", err)
	}
}

func TestExec(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.41/containers/side/exec":
			var body struct{ Cmd []string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Cmd[0] != "nvidia-smi" {
				t.Errorf("cmd = %v", body.Cmd)
			}
			_, _ = io.WriteString(w, `{"Id":"e1"}`)
		case "/v1.41/exec/e1/start":
			_, _ = w.Write(frame(1, "0, GPU, 70\n"))
		case "/v1.41/exec/e1/json":
			_, _ = io.WriteString(w, `{"ExitCode":0}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	out, err := c.Exec(context.Background(), "side", []string{"nvidia-smi"})
	if err != nil || out != "0, GPU, 70\n" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestExecNonZeroExit(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/exec"):
			_, _ = io.WriteString(w, `{"Id":"e2"}`)
		case strings.HasSuffix(r.URL.Path, "/start"):
			_, _ = w.Write(frame(2, "NVIDIA-SMI has failed"))
		default:
			_, _ = io.WriteString(w, `{"ExitCode":9}`)
		}
	})
	_, err := c.Exec(context.Background(), "side", []string{"nvidia-smi"})
	if err == nil || !strings.Contains(err.Error(), "NVIDIA-SMI has failed") {
		t.Fatalf("err = %v", err)
	}
}
