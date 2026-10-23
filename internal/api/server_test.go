package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNovice/docker-gpu-governor/internal/governor"
)

type fakeGovernor struct {
	cfg       governor.Config
	resumed   bool
	resumeErr error
}

func (f *fakeGovernor) Status() governor.Status { return governor.Status{Hot: true, Config: f.cfg} }
func (f *fakeGovernor) Config() governor.Config { return f.cfg }

func (f *fakeGovernor) SetConfig(c governor.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	f.cfg = c
	return nil
}

func (f *fakeGovernor) ResumeAll(context.Context) error {
	f.resumed = true
	return f.resumeErr
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestStatus(t *testing.T) {
	h := Handler(&fakeGovernor{cfg: governor.DefaultConfig()})
	rec := do(t, h, http.MethodGet, "/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var st governor.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || !st.Hot {
		t.Fatalf("st=%+v err=%v", st, err)
	}
}

func TestSetConfig(t *testing.T) {
	g := &fakeGovernor{cfg: governor.DefaultConfig()}
	h := Handler(g)

	cfg := governor.DefaultConfig()
	cfg.GPU.PauseAtC = 80
	body, _ := json.Marshal(cfg)
	if rec := do(t, h, http.MethodPost, "/config", string(body)); rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body)
	}
	if g.cfg.GPU.PauseAtC != 80 {
		t.Fatal("config not applied")
	}

	cfg.GPU.ResumeAtC = 90
	body, _ = json.Marshal(cfg)
	rec := do(t, h, http.MethodPost, "/config", string(body))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "resumeAtC") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}

	if rec := do(t, h, http.MethodPost, "/config", `{"enabled":true,"typo":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields must be rejected, code = %d", rec.Code)
	}
}

func TestResume(t *testing.T) {
	g := &fakeGovernor{cfg: governor.DefaultConfig()}
	h := Handler(g)
	if rec := do(t, h, http.MethodPost, "/resume", ""); rec.Code != http.StatusOK || !g.resumed {
		t.Fatalf("code=%d resumed=%v", rec.Code, g.resumed)
	}
	g.resumeErr = errors.New("daemon down")
	if rec := do(t, h, http.MethodPost, "/resume", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := Handler(&fakeGovernor{})
	if rec := do(t, h, http.MethodDelete, "/status", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d", rec.Code)
	}
}
