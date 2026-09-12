package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/spf13/cobra"
)

// fakeInstancesAPI serves the GetMac instance list and delete endpoints.
type fakeInstancesAPI struct {
	mu           sync.Mutex
	instances    []map[string]string
	deleteStatus int
	deleted      []string
}

func (f *fakeInstancesAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/instances":
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(f.instances), "instances": f.instances})
	case r.Method == http.MethodDelete:
		f.deleted = append(f.deleted, path.Base(r.URL.Path))
		if f.deleteStatus != 0 {
			w.WriteHeader(f.deleteStatus)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Instance not found"})
			return
		}
		_ = json.NewEncoder(w).Encode("Instance deleted successfully")
	default:
		http.NotFound(w, r)
	}
}

func setStageEnv(t *testing.T, api http.Handler) {
	t.Helper()

	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_API_URL", srv.URL)
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_API_KEY", "key")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID", "project")
	t.Setenv("CUSTOM_ENV_CI_JOB_ID", "7")
	t.Setenv("CUSTOM_ENV_CI_JOB_URL", "https://gitlab.example.com/jobs/7")
}

func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func TestCleanup_DeletesVirtualMachine(t *testing.T) {
	api := &fakeInstancesAPI{instances: []map[string]string{{"id": "vm-7", "name": "gitlab-job-7"}}}
	setStageEnv(t, api)

	if err := runCleanupCommand(newTestCommand(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.deleted) != 1 || api.deleted[0] != "vm-7" {
		t.Fatalf("expected vm-7 to be deleted, got %v", api.deleted)
	}
}

func TestCleanup_NoVirtualMachineIsNotAnError(t *testing.T) {
	api := &fakeInstancesAPI{instances: []map[string]string{{"id": "vm-8", "name": "gitlab-job-8"}}}
	setStageEnv(t, api)

	if err := runCleanupCommand(newTestCommand(), nil); err != nil {
		t.Fatalf("expected cleanup to succeed without a virtual machine, got %v", err)
	}

	if len(api.deleted) != 0 {
		t.Fatalf("expected nothing to be deleted, got %v", api.deleted)
	}
}

func TestCleanup_AlreadyDeletedIsNotAnError(t *testing.T) {
	api := &fakeInstancesAPI{
		instances:    []map[string]string{{"id": "vm-7", "name": "gitlab-job-7"}},
		deleteStatus: http.StatusNotFound,
	}
	setStageEnv(t, api)

	if err := runCleanupCommand(newTestCommand(), nil); err != nil {
		t.Fatalf("expected cleanup to succeed when the virtual machine is already gone, got %v", err)
	}
}

func TestRun_MissingVirtualMachineIsSystemFailure(t *testing.T) {
	setStageEnv(t, &fakeInstancesAPI{})

	script := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(script, []byte("echo hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := runRunCommand(newTestCommand(), []string{script})

	var systemFailure *gitlab.SystemFailureError
	if !errors.As(err, &systemFailure) {
		t.Fatalf("expected a system failure, got %v", err)
	}
}
