package sensor

import (
	"context"
	"errors"
	"testing"
)

func TestParseNvidiaSMI(t *testing.T) {
	out := "0, NVIDIA GeForce RTX 4090 Laptop GPU, 63\n1, Tesla T4, [N/A]\n2, NVIDIA A100-SXM4-80GB, 81\n"
	got, err := ParseNvidiaSMI(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[1].Name != "GPU 2 NVIDIA A100-SXM4-80GB" || got[1].TempC != 81 {
		t.Fatalf("got %+v", got[1])
	}
}

func TestParseNvidiaSMIKeepsCommasInName(t *testing.T) {
	got, err := ParseNvidiaSMI("0, Some, Odd Name, 50")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "GPU 0 Some, Odd Name" {
		t.Fatalf("name = %q", got[0].Name)
	}
}

func TestParseNvidiaSMIRejectsGarbage(t *testing.T) {
	if _, err := ParseNvidiaSMI("No devices were found"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := ParseNvidiaSMI(""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestGPUUsesRunner(t *testing.T) {
	var argv []string
	run := func(_ context.Context, a []string) (string, error) {
		argv = a
		return "0, GPU, 70\n", nil
	}
	got, err := GPU(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if argv[0] != "nvidia-smi" || got[0].TempC != 70 {
		t.Fatalf("argv=%v got=%+v", argv, got)
	}
}
