// Package docker is a small client for the parts of the Docker Engine API
// the governor needs. It talks to the daemon over its unix socket so the
// binary has no dependencies outside the standard library.
package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiVersion is the oldest Engine API version that has DeviceRequests (20.10).
const apiVersion = "v1.41"

// ErrNotFound is returned when the daemon answers 404.
var ErrNotFound = errors.New("not found")

// Client calls the Docker Engine API.
type Client struct {
	http *http.Client
	base string
}

// New returns a client for the daemon listening on socketPath.
func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		http: &http.Client{Transport: transport, Timeout: 30 * time.Second},
		base: "http://docker",
	}
}

// NewWithHTTP returns a client that sends requests to baseURL. Used by tests.
func NewWithHTTP(baseURL string, hc *http.Client) *Client {
	return &Client{http: hc, base: strings.TrimSuffix(baseURL, "/")}
}

// Container is the subset of container state the governor uses.
type Container struct {
	ID     string
	Name   string
	Image  string
	State  string // "running", "paused", "exited", ...
	Labels map[string]string
}

// Details adds fields that only the inspect endpoint returns.
type Details struct {
	Container
	UsesGPU bool
}

// ListContainers returns running and paused containers.
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	filters := url.QueryEscape(`{"status":["running","paused"]}`)
	var raw []struct {
		ID     string `json:"Id"`
		Names  []string
		Image  string
		State  string
		Labels map[string]string
	}
	if err := c.do(ctx, http.MethodGet, "/containers/json?filters="+filters, nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		name := r.ID[:min(12, len(r.ID))]
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, Container{ID: r.ID, Name: name, Image: r.Image, State: r.State, Labels: r.Labels})
	}
	return out, nil
}

// Inspect returns details for one container.
func (c *Client) Inspect(ctx context.Context, id string) (Details, error) {
	var raw struct {
		ID    string `json:"Id"`
		Name  string
		State struct {
			Status string
		}
		Config struct {
			Image  string
			Labels map[string]string
		}
		HostConfig struct {
			Runtime        string
			DeviceRequests []struct {
				Driver       string
				Capabilities [][]string
			}
		}
	}
	if err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, &raw); err != nil {
		return Details{}, err
	}
	d := Details{Container: Container{
		ID:     raw.ID,
		Name:   strings.TrimPrefix(raw.Name, "/"),
		Image:  raw.Config.Image,
		State:  raw.State.Status,
		Labels: raw.Config.Labels,
	}}
	if raw.HostConfig.Runtime == "nvidia" {
		d.UsesGPU = true
	}
	for _, req := range raw.HostConfig.DeviceRequests {
		if req.Driver == "nvidia" {
			d.UsesGPU = true
		}
		for _, caps := range req.Capabilities {
			for _, capability := range caps {
				if capability == "gpu" {
					d.UsesGPU = true
				}
			}
		}
	}
	return d, nil
}

// Pause freezes every process in the container. Memory, open files and GPU
// allocations are kept, so the workload continues where it stopped on Unpause.
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/pause", nil, nil)
}

// Unpause thaws a paused container.
func (c *Client) Unpause(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/unpause", nil, nil)
}

// CreateSpec describes a container to create.
type CreateSpec struct {
	Name       string
	Image      string
	Entrypoint []string
	Cmd        []string
	Env        []string
	Labels     map[string]string
	GPUs       bool
	AutoRemove bool
}

// Create creates a container and returns its ID.
func (c *Client) Create(ctx context.Context, spec CreateSpec) (string, error) {
	type deviceRequest struct {
		Driver       string
		Count        int
		Capabilities [][]string
	}
	hostConfig := map[string]any{"AutoRemove": spec.AutoRemove}
	if spec.GPUs {
		hostConfig["DeviceRequests"] = []deviceRequest{{Count: -1, Capabilities: [][]string{{"gpu"}}}}
	}
	body := map[string]any{
		"Image":      spec.Image,
		"Entrypoint": spec.Entrypoint,
		"Cmd":        spec.Cmd,
		"Env":        spec.Env,
		"Labels":     spec.Labels,
		"HostConfig": hostConfig,
	}
	var resp struct {
		ID string `json:"Id"`
	}
	path := "/containers/create?name=" + url.QueryEscape(spec.Name)
	if err := c.do(ctx, http.MethodPost, path, body, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// Start starts a created container.
func (c *Client) Start(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, nil)
}

// Remove force-removes a container.
func (c *Client) Remove(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id)+"?force=true", nil, nil)
}

// Exec runs argv inside a running container and returns its standard output.
// A non-zero exit code is reported as an error that includes standard error.
func (c *Client) Exec(ctx context.Context, id string, argv []string) (string, error) {
	var created struct {
		ID string `json:"Id"`
	}
	createBody := map[string]any{"AttachStdout": true, "AttachStderr": true, "Cmd": argv}
	if err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/exec", createBody, &created); err != nil {
		return "", err
	}

	resp, err := c.send(ctx, http.MethodPost, "/exec/"+created.ID+"/start", map[string]any{"Detach": false, "Tty": false})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var stdout, stderr bytes.Buffer
	if err := demux(resp.Body, &stdout, &stderr); err != nil {
		return "", fmt.Errorf("read exec output: %w", err)
	}

	var inspect struct {
		ExitCode int
	}
	if err := c.do(ctx, http.MethodGet, "/exec/"+created.ID+"/json", nil, &inspect); err != nil {
		return "", err
	}
	if inspect.ExitCode != 0 {
		return stdout.String(), fmt.Errorf("exit code %d: %s", inspect.ExitCode, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// demux splits the stream format used by attach and exec when no TTY is
// allocated: an 8 byte header (stream id, 3 zero bytes, big endian length)
// followed by the payload.
func demux(r io.Reader, stdout, stderr io.Writer) error {
	var header [8]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		size := int64(binary.BigEndian.Uint32(header[4:]))
		dst := stdout
		if header[0] == 2 {
			dst = stderr
		}
		if _, err := io.CopyN(dst, r, size); err != nil {
			return err
		}
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/"+apiVersion+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var apiErr struct{ Message string }
		if json.Unmarshal(msg, &apiErr) == nil && apiErr.Message != "" {
			msg = []byte(apiErr.Message)
		}
		err := fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
		if resp.StatusCode == http.StatusNotFound {
			err = fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return nil, err
	}
	return resp, nil
}
