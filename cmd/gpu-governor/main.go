// Command gpu-governor pauses Docker containers while the CPU or GPU is too
// hot and resumes them once temperatures are back in range.
//
// Usage:
//
//	gpu-governor [serve] [flags]   run the governor and its HTTP API
//	gpu-governor idle              block until stopped (used by the GPU probe)
//	gpu-governor version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNovice/docker-gpu-governor/internal/api"
	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
	"github.com/QuantumNovice/docker-gpu-governor/internal/governor"
	"github.com/QuantumNovice/docker-gpu-governor/internal/probe"
	"github.com/QuantumNovice/docker-gpu-governor/internal/sensor"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "idle":
		idle()
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gpu-governor:", err)
		os.Exit(1)
	}
}

func idle() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type options struct {
	socket       string
	listen       string
	dockerSocket string
	statePath    string
	sysRoot      string
	image        string
	gpuSource    string
}

func serve(args []string) error {
	var o options
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&o.socket, "socket", env("GOVERNOR_SOCKET", ""), "serve the API on this unix socket")
	fs.StringVar(&o.listen, "listen", env("GOVERNOR_LISTEN", ""), "serve the API on this TCP address (default :9090 when -socket is not set)")
	fs.StringVar(&o.dockerSocket, "docker-socket", env("DOCKER_SOCKET", "/var/run/docker.sock"), "Docker daemon socket")
	fs.StringVar(&o.statePath, "state", env("GOVERNOR_STATE", "/var/lib/gpu-governor/state.json"), "file that keeps settings and paused containers")
	fs.StringVar(&o.sysRoot, "sys", env("GOVERNOR_SYS", "/sys"), "sysfs root used for CPU temperatures")
	fs.StringVar(&o.image, "image", env("GOVERNOR_IMAGE", ""), "image for the GPU probe container (default: the image this container runs)")
	fs.StringVar(&o.gpuSource, "gpu", env("GOVERNOR_GPU", "auto"), "GPU source: auto, local, probe or off")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.socket == "" && o.listen == "" {
		o.listen = ":9090"
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	engine := docker.New(o.dockerSocket)
	samp := &sampler{sysRoot: o.sysRoot}
	var gpuProbe *probe.Probe
	switch src := o.gpuSource; {
	case src == "off":
	case src == "local" || (src == "auto" && sensor.HasLocalNvidiaSMI()):
		samp.gpu = func(ctx context.Context) ([]sensor.Reading, error) {
			return sensor.GPU(ctx, sensor.LocalRunner)
		}
	case src == "probe" || src == "auto":
		image, err := probeImage(ctx, engine, o.image)
		if err != nil {
			log.Warn("GPU probe disabled", "err", err)
			break
		}
		gpuProbe = probe.New(engine, image)
		samp.gpu = gpuProbe.GPU
	default:
		return fmt.Errorf("unknown -gpu value %q", src)
	}

	gov := governor.New(engine, samp, governor.DefaultConfig(), log)
	if err := gov.Restore(governor.FileStore{Path: o.statePath}); err != nil {
		log.Warn("could not load saved state, starting fresh", "err", err)
	}

	ln, err := listen(o)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: api.Handler(gov), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api server stopped", "err", err)
			stop()
		}
	}()
	log.Info("gpu-governor started", "version", version, "api", ln.Addr().String())

	gov.Run(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := gov.Shutdown(shutdownCtx); err != nil {
		log.Error("some containers could not be resumed", "err", err)
	}
	if gpuProbe != nil {
		_ = gpuProbe.Close(shutdownCtx)
	}
	_ = srv.Shutdown(shutdownCtx)
	log.Info("gpu-governor stopped")
	return nil
}

func listen(o options) (net.Listener, error) {
	if o.socket != "" {
		_ = os.Remove(o.socket)
		return net.Listen("unix", o.socket)
	}
	return net.Listen("tcp", o.listen)
}

// probeImage returns the image to run the GPU probe with. Inside a container
// the hostname is the container ID, which gives us our own image.
func probeImage(ctx context.Context, engine *docker.Client, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	self, err := engine.Inspect(ctx, host)
	if err != nil {
		return "", fmt.Errorf("set GOVERNOR_IMAGE or run inside a container: %w", err)
	}
	return self.Image, nil
}

type sampler struct {
	sysRoot string
	gpu     func(context.Context) ([]sensor.Reading, error)
}

func (s *sampler) CPU(context.Context) ([]sensor.Reading, error) {
	return sensor.CPU(s.sysRoot)
}

func (s *sampler) GPU(ctx context.Context) ([]sensor.Reading, error) {
	if s.gpu == nil {
		return nil, sensor.ErrUnavailable
	}
	return s.gpu(ctx)
}
