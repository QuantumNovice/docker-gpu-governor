package governor

import "github.com/QuantumNovice/docker-gpu-governor/internal/docker"

// Container labels understood by the governor.
const (
	// LabelEnable opts a container in regardless of the selection mode.
	LabelEnable = "gpu-governor.enable"
	// LabelExclude opts a container out regardless of the selection mode.
	LabelExclude = "gpu-governor.exclude"
	// LabelRole marks containers the governor itself runs.
	LabelRole = "gpu-governor.role"

	// Docker Desktop extension images carry this label, and their containers
	// inherit it. Pausing another extension's backend would break its UI.
	labelExtensionAPI = "com.docker.desktop.extension.api.version"
)

// Selection modes.
const (
	ModeGPU     = "gpu"     // containers started with --gpus or the nvidia runtime
	ModeLabeled = "labeled" // only containers labeled gpu-governor.enable=true
	ModeAll     = "all"     // every running container
)

// ValidMode reports whether m is a known selection mode.
func ValidMode(m string) bool {
	return m == ModeGPU || m == ModeLabeled || m == ModeAll
}

// IsTarget decides whether the governor may pause a container.
func IsTarget(mode string, c docker.Container, usesGPU bool) bool {
	labels := c.Labels
	if labels[LabelRole] != "" || labels[labelExtensionAPI] != "" {
		return false
	}
	if isTrue(labels[LabelExclude]) {
		return false
	}
	if isTrue(labels[LabelEnable]) {
		return true
	}
	switch mode {
	case ModeAll:
		return true
	case ModeGPU:
		return usesGPU
	default:
		return false
	}
}

func isTrue(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}
