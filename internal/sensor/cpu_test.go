package sensor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCPUFromHwmon(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon0/name"), "nvme\n")
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon0/temp1_input"), "99000\n")
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon1/name"), "coretemp\n")
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon1/temp1_input"), "61000\n")
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon1/temp1_label"), "Package id 0\n")
	writeFile(t, filepath.Join(root, "class/hwmon/hwmon1/temp2_input"), "72500\n")

	got, err := CPU(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d readings, want 2 (nvme must be skipped): %+v", len(got), got)
	}
	hot, _ := Max(got)
	if hot.TempC != 72.5 || hot.Name != "coretemp temp2" {
		t.Fatalf("hottest = %+v", hot)
	}
}

func TestCPUFallsBackToThermalZones(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "class/thermal/thermal_zone0/type"), "acpitz\n")
	writeFile(t, filepath.Join(root, "class/thermal/thermal_zone0/temp"), "40000\n")
	writeFile(t, filepath.Join(root, "class/thermal/thermal_zone1/type"), "x86_pkg_temp\n")
	writeFile(t, filepath.Join(root, "class/thermal/thermal_zone1/temp"), "55000\n")

	got, err := CPU(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TempC != 55 {
		t.Fatalf("got %+v", got)
	}
}

func TestCPUUnavailable(t *testing.T) {
	if _, err := CPU(t.TempDir()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
