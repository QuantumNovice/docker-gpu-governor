package sensor

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// NvidiaSMIArgs is the query passed to nvidia-smi. The output is parsed by
// ParseNvidiaSMI.
var NvidiaSMIArgs = []string{
	"--query-gpu=index,name,temperature.gpu",
	"--format=csv,noheader,nounits",
}

// CommandRunner runs a command and returns its standard output.
type CommandRunner func(ctx context.Context, argv []string) (string, error)

// GPU reads NVIDIA GPU temperatures by running nvidia-smi through run.
func GPU(ctx context.Context, run CommandRunner) ([]Reading, error) {
	out, err := run(ctx, append([]string{"nvidia-smi"}, NvidiaSMIArgs...))
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi: %w", err)
	}
	return ParseNvidiaSMI(out)
}

// ParseNvidiaSMI parses lines such as "0, NVIDIA GeForce RTX 4090, 63".
// GPUs that report "[N/A]" for temperature are skipped.
func ParseNvidiaSMI(out string) ([]Reading, error) {
	var readings []Reading
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			return nil, fmt.Errorf("unexpected nvidia-smi line %q", line)
		}
		index := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(strings.Join(fields[1:len(fields)-1], ","))
		temp, err := strconv.ParseFloat(strings.TrimSpace(fields[len(fields)-1]), 64)
		if err != nil {
			continue
		}
		readings = append(readings, Reading{Name: fmt.Sprintf("GPU %s %s", index, name), TempC: temp})
	}
	if len(readings) == 0 {
		return nil, ErrUnavailable
	}
	return readings, nil
}

// LocalRunner runs commands on the current machine.
func LocalRunner(ctx context.Context, argv []string) (string, error) {
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	return string(out), err
}

// HasLocalNvidiaSMI reports whether nvidia-smi is on PATH.
func HasLocalNvidiaSMI() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}
