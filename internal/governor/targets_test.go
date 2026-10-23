package governor

import (
	"testing"

	"github.com/QuantumNovice/docker-gpu-governor/internal/docker"
)

func TestIsTarget(t *testing.T) {
	plain := docker.Container{Labels: map[string]string{}}
	enabled := docker.Container{Labels: map[string]string{LabelEnable: "true"}}
	excluded := docker.Container{Labels: map[string]string{LabelExclude: "true", LabelEnable: "true"}}
	own := docker.Container{Labels: map[string]string{LabelRole: "probe"}}
	extension := docker.Container{Labels: map[string]string{labelExtensionAPI: ">= 0.3.4"}}

	cases := []struct {
		name    string
		mode    string
		c       docker.Container
		usesGPU bool
		want    bool
	}{
		{"gpu mode picks gpu containers", ModeGPU, plain, true, true},
		{"gpu mode skips cpu containers", ModeGPU, plain, false, false},
		{"label opts in under gpu mode", ModeGPU, enabled, false, true},
		{"labeled mode needs the label", ModeLabeled, plain, true, false},
		{"labeled mode honours the label", ModeLabeled, enabled, false, true},
		{"all mode takes everything", ModeAll, plain, false, true},
		{"exclude wins over enable", ModeAll, excluded, true, false},
		{"never pauses itself", ModeAll, own, true, false},
		{"never pauses extensions", ModeAll, extension, true, false},
		{"unknown mode selects nothing", "bogus", plain, true, false},
	}
	for _, tc := range cases {
		if got := IsTarget(tc.mode, tc.c, tc.usesGPU); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
