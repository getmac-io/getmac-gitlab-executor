package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getmac-io/getmac-sdk-golang"
)

type fakeVMResponse struct {
	code   int
	status string
	reason string
}

// newFakeAPI serves GET /instances/vm-1 with the given responses in order,
// repeating the last one once they run out.
func newFakeAPI(t *testing.T, responses ...fakeVMResponse) (*getmac.Client, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/instances/vm-1" || r.URL.Query().Get("project_id") != "project-1" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}

		n := int(calls.Add(1)) - 1
		if n >= len(responses) {
			n = len(responses) - 1
		}

		res := responses[n]
		if res.code != 0 && res.code != http.StatusOK {
			w.WriteHeader(res.code)
			return
		}

		_ = json.NewEncoder(w).Encode(getmac.VirtualMachine{ID: "vm-1", Status: res.status, StatusReason: res.reason})
	}))
	t.Cleanup(srv.Close)

	return getmac.NewClient(getmac.WithBaseURL(srv.URL), getmac.WithToken("token")), &calls
}

func testWaitOptions() waitOptions {
	return waitOptions{Timeout: 2 * time.Second, PollInterval: time.Millisecond, RequestTimeout: time.Second}
}

func TestWaitForVirtualMachineRunning_ReturnsOnceRunning(t *testing.T) {
	client, calls := newFakeAPI(t,
		fakeVMResponse{status: "queued", reason: "Project instance count limit exceeded: 3/3"},
		fakeVMResponse{status: "pending"},
		fakeVMResponse{status: "creating"},
		fakeVMResponse{status: "starting"},
		fakeVMResponse{status: "running"},
	)

	vm, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", testWaitOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vm.Status != "running" {
		t.Fatalf("expected status running, got %q", vm.Status)
	}

	if got := calls.Load(); got != 5 {
		t.Fatalf("expected 5 status requests, got %d", got)
	}
}

func TestWaitForVirtualMachineRunning_RetriesServerErrors(t *testing.T) {
	client, calls := newFakeAPI(t,
		fakeVMResponse{code: http.StatusServiceUnavailable},
		fakeVMResponse{code: http.StatusTooManyRequests},
		fakeVMResponse{status: "running"},
	)

	if _, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", testWaitOptions()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 status requests, got %d", got)
	}
}

func TestWaitForVirtualMachineRunning_FailsOnFailedStatus(t *testing.T) {
	client, _ := newFakeAPI(t,
		fakeVMResponse{status: "starting"},
		fakeVMResponse{status: "deleting"},
	)

	_, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", testWaitOptions())
	if err == nil || !strings.Contains(err.Error(), "is deleting") {
		t.Fatalf("expected deleting error, got %v", err)
	}
}

func TestWaitForVirtualMachineRunning_FailsWhenVirtualMachineIsGone(t *testing.T) {
	client, calls := newFakeAPI(t, fakeVMResponse{code: http.StatusNotFound})

	_, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", testWaitOptions())
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("expected not found error, got %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 status request, got %d", got)
	}
}

func TestWaitForVirtualMachineRunning_FailsOnClientError(t *testing.T) {
	client, calls := newFakeAPI(t, fakeVMResponse{code: http.StatusUnauthorized})

	_, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", testWaitOptions())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected unauthorized error, got %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 status request, got %d", got)
	}
}

func TestWaitForVirtualMachineRunning_TimesOutWithLastStatus(t *testing.T) {
	client, _ := newFakeAPI(t, fakeVMResponse{status: "queued", reason: "Project instance minutes limit exceeded: 3004/3000"})

	opts := testWaitOptions()
	opts.Timeout = 50 * time.Millisecond
	opts.PollInterval = 5 * time.Millisecond

	_, err := waitForVirtualMachineRunning(context.Background(), client, "project-1", "vm-1", opts)
	if err == nil || !strings.Contains(err.Error(), "last status: queued (Project instance minutes limit exceeded: 3004/3000)") {
		t.Fatalf("expected timeout error with last status, got %v", err)
	}
}

func TestWaitForVirtualMachineRunning_StopsWhenCancelled(t *testing.T) {
	client, _ := newFakeAPI(t, fakeVMResponse{status: "starting"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := waitForVirtualMachineRunning(ctx, client, "project-1", "vm-1", testWaitOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
