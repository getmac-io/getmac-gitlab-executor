package gitlab

import (
	"strings"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()

	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_API_KEY", "key")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_PROJECT_ID", "project")
	t.Setenv("CUSTOM_ENV_CI_JOB_ID", "1")
	t.Setenv("CUSTOM_ENV_CI_JOB_URL", "https://gitlab.example.com/jobs/1")
}

func TestNewEnvironment_ReadyTimeoutDefaults(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_VM_READY_TIMEOUT", "")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_SSH_READY_TIMEOUT", "")

	env, err := NewEnvironment()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.VMReadyTimeout != DefaultVMReadyTimeout {
		t.Errorf("expected VM ready timeout %s, got %s", DefaultVMReadyTimeout, env.VMReadyTimeout)
	}

	if env.SSHReadyTimeout != DefaultSSHReadyTimeout {
		t.Errorf("expected SSH ready timeout %s, got %s", DefaultSSHReadyTimeout, env.SSHReadyTimeout)
	}
}

func TestNewEnvironment_ReadyTimeoutOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_VM_READY_TIMEOUT", "45m")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_SSH_READY_TIMEOUT", "90s")

	env, err := NewEnvironment()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.VMReadyTimeout != 45*time.Minute {
		t.Errorf("expected VM ready timeout 45m, got %s", env.VMReadyTimeout)
	}

	if env.SSHReadyTimeout != 90*time.Second {
		t.Errorf("expected SSH ready timeout 90s, got %s", env.SSHReadyTimeout)
	}
}

func TestNewEnvironment_RejectsInvalidReadyTimeouts(t *testing.T) {
	for _, key := range []string{"GETMAC_CLOUD_VM_READY_TIMEOUT", "GETMAC_CLOUD_SSH_READY_TIMEOUT"} {
		for _, value := range []string{"soon", "0s", "-1m"} {
			t.Run(key+"="+value, func(t *testing.T) {
				setRequiredEnv(t)
				t.Setenv(EnvironmentPrefix+key, value)

				_, err := NewEnvironment()
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("expected error naming %s, got %v", key, err)
				}
			})
		}
	}
}

func TestNewEnvironment_MachineDefaults(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_MACHINE_IMAGE", "")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_MACHINE_TYPE", "")

	env, err := NewEnvironment()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.MachineImage != "getmac" {
		t.Errorf("expected the getmac label by default, got %q", env.MachineImage)
	}

	if env.MachineType != "" {
		t.Errorf("expected no default machine type so the label decides it, got %q", env.MachineType)
	}
}

func TestNewEnvironment_MachineOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_MACHINE_IMAGE", " getmac-tahoe ")
	t.Setenv("CUSTOM_ENV_GETMAC_CLOUD_MACHINE_TYPE", "mac-m4-c4-m8")

	env, err := NewEnvironment()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.MachineImage != "getmac-tahoe" {
		t.Errorf("expected image getmac-tahoe, got %q", env.MachineImage)
	}

	if env.MachineType != "mac-m4-c4-m8" {
		t.Errorf("expected type mac-m4-c4-m8, got %q", env.MachineType)
	}
}
