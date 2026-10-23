package sensor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// hwmon drivers that report package or core temperatures.
var cpuHwmonDrivers = map[string]bool{
	"coretemp":    true, // Intel
	"k10temp":     true, // AMD
	"zenpower":    true, // AMD, out of tree
	"cpu_thermal": true, // Raspberry Pi and other ARM boards
	"soc_thermal": true,
}

// thermal zone types that describe the CPU package.
var cpuZoneTypes = map[string]bool{
	"x86_pkg_temp": true,
	"cpu-thermal":  true,
	"cpu_thermal":  true,
	"soc_thermal":  true,
	"soc-thermal":  true,
}

// CPU reads CPU temperatures from sysfs under sysRoot (normally /sys).
// hwmon is tried first because it gives per-core values; thermal zones are
// the fallback for boards that only expose those.
func CPU(sysRoot string) ([]Reading, error) {
	readings := cpuFromHwmon(sysRoot)
	if len(readings) == 0 {
		readings = cpuFromThermalZones(sysRoot)
	}
	if len(readings) == 0 {
		return nil, ErrUnavailable
	}
	return readings, nil
}

func cpuFromHwmon(sysRoot string) []Reading {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class", "hwmon", "hwmon*"))
	var out []Reading
	for _, dir := range dirs {
		driver := readTrimmed(filepath.Join(dir, "name"))
		if !cpuHwmonDrivers[driver] {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
		for _, input := range inputs {
			milli, err := readInt(input)
			if err != nil {
				continue
			}
			label := readTrimmed(strings.TrimSuffix(input, "_input") + "_label")
			if label == "" {
				label = strings.TrimSuffix(filepath.Base(input), "_input")
			}
			out = append(out, Reading{
				Name:  fmt.Sprintf("%s %s", driver, label),
				TempC: float64(milli) / 1000,
			})
		}
	}
	return out
}

func cpuFromThermalZones(sysRoot string) []Reading {
	zones, _ := filepath.Glob(filepath.Join(sysRoot, "class", "thermal", "thermal_zone*"))
	var out []Reading
	for _, zone := range zones {
		kind := readTrimmed(filepath.Join(zone, "type"))
		if !cpuZoneTypes[kind] {
			continue
		}
		milli, err := readInt(filepath.Join(zone, "temp"))
		if err != nil {
			continue
		}
		out = append(out, Reading{Name: kind, TempC: float64(milli) / 1000})
	}
	return out
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readInt(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}
