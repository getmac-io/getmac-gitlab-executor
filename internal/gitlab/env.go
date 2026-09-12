package gitlab

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvironmentPrefix = "CUSTOM_ENV_"

	// DefaultMachineImage is the GetMac runner label used when
	// GETMAC_CLOUD_MACHINE_IMAGE is unset, the same label as runs-on: getmac in
	// GitHub Actions.
	DefaultMachineImage = "getmac"

	DefaultVMReadyTimeout  = 20 * time.Minute
	DefaultSSHReadyTimeout = 5 * time.Minute
)

type RunnerConfig struct {
	BuildsDir         string             `json:"builds_dir"`
	CacheDir          string             `json:"cache_dir"`
	BuildsDirIsShared bool               `json:"builds_dir_is_shared"`
	Hostname          string             `json:"hostname"`
	JobEnv            map[string]string  `json:"job_env"`
	Driver            RunnerDriverConfig `json:"driver"`
	Shell             string             `json:"shell"`
}

type RunnerDriverConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Environment struct {
	URL                string
	Token              string
	ProjectID          string
	MachineImage       string
	MachineType        string
	Region             string
	SSHPrivateKeyPath  string
	JobID              string
	JobURL             string
	Debug              bool
	ProxyTunnelEnabled bool
	ProxyTunnelPort    int
	VMReadyTimeout     time.Duration
	SSHReadyTimeout    time.Duration
}

func lookupEnv(key string) (string, bool) {
	return os.LookupEnv(fmt.Sprintf("%s%s", EnvironmentPrefix, key))
}

func lookupDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	raw, ok := lookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}

	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %v", key, err)
	}

	if value <= 0 {
		return 0, fmt.Errorf("invalid value for %s: must be greater than zero", key)
	}

	return value, nil
}

func NewEnvironment() (*Environment, error) {
	var ok bool
	env := &Environment{}

	env.URL, _ = lookupEnv("GETMAC_CLOUD_API_URL")
	if env.URL == "" {
		env.URL = "https://api.getmac.io/v1"
	}

	if env.Token, ok = lookupEnv("GETMAC_CLOUD_API_KEY"); !ok || strings.TrimSpace(env.Token) == "" {
		return nil, fmt.Errorf("missing required environment variable: GETMAC_CLOUD_API_KEY")
	}

	if env.ProjectID, ok = lookupEnv("GETMAC_CLOUD_PROJECT_ID"); !ok || strings.TrimSpace(env.ProjectID) == "" {
		return nil, fmt.Errorf("missing required environment variable: GETMAC_CLOUD_PROJECT_ID")
	}

	// A GetMac runner label (the names GitHub Actions uses in runs-on) or an image
	// slug. The API resolves a label to its image and machine type.
	env.MachineImage, _ = lookupEnv("GETMAC_CLOUD_MACHINE_IMAGE")
	env.MachineImage = strings.TrimSpace(env.MachineImage)
	if env.MachineImage == "" {
		env.MachineImage = DefaultMachineImage
	}

	// No default, so the label in GETMAC_CLOUD_MACHINE_IMAGE decides the machine
	// type. An image slug needs an explicit type.
	env.MachineType, _ = lookupEnv("GETMAC_CLOUD_MACHINE_TYPE")
	env.MachineType = strings.TrimSpace(env.MachineType)

	env.Region, _ = lookupEnv("GETMAC_CLOUD_REGION")
	if env.Region == "" {
		env.Region = "eu-central-ltu-1"
	}

	if env.JobID, ok = lookupEnv("CI_JOB_ID"); !ok || strings.TrimSpace(env.JobID) == "" {
		return nil, fmt.Errorf("missing required environment variable: CI_JOB_ID")
	}

	if env.JobURL, ok = lookupEnv("CI_JOB_URL"); !ok || strings.TrimSpace(env.JobURL) == "" {
		return nil, fmt.Errorf("missing required environment variable: CI_JOB_URL")
	}

	env.SSHPrivateKeyPath, _ = lookupEnv("GETMAC_CLOUD_SSH_PRIVATE_KEY_PATH")
	if env.SSHPrivateKeyPath == "" {
		env.SSHPrivateKeyPath = fmt.Sprintf("%s/.ssh/id_rsa", os.Getenv("HOME"))
	}

	if debugStr, ok := lookupEnv("GETMAC_CLOUD_DEBUG"); ok {
		debug, err := strconv.ParseBool(debugStr)
		if err != nil {
			return nil, fmt.Errorf("invalid value for GETMAC_CLOUD_DEBUG: %v", err)
		}

		env.Debug = debug
	}

	if ptStr, ok := lookupEnv("GETMAC_PROXY_TUNNEL_ENABLED"); ok {
		enabled, err := strconv.ParseBool(ptStr)
		if err != nil {
			return nil, fmt.Errorf("invalid value for GETMAC_PROXY_TUNNEL_ENABLED: %v", err)
		}
		env.ProxyTunnelEnabled = enabled
	}

	env.ProxyTunnelPort = 8080
	if portStr, ok := lookupEnv("GETMAC_PROXY_TUNNEL_PORT"); ok {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid value for GETMAC_PROXY_TUNNEL_PORT: %v", err)
		}
		env.ProxyTunnelPort = port
	}

	var err error
	if env.VMReadyTimeout, err = lookupDuration("GETMAC_CLOUD_VM_READY_TIMEOUT", DefaultVMReadyTimeout); err != nil {
		return nil, err
	}

	if env.SSHReadyTimeout, err = lookupDuration("GETMAC_CLOUD_SSH_READY_TIMEOUT", DefaultSSHReadyTimeout); err != nil {
		return nil, err
	}

	return env, nil
}

func GetBuildFailureExitCode() (int, error) {
	exitCodeRaw, ok := os.LookupEnv("BUILD_FAILURE_EXIT_CODE")
	if !ok || strings.TrimSpace(exitCodeRaw) == "" {
		return 1, nil
	}

	exitCode, err := strconv.Atoi(exitCodeRaw)
	if err != nil {
		return 0, fmt.Errorf("invalid BUILD_FAILURE_EXIT_CODE value: %v", err)
	}

	return exitCode, nil
}

func GetSystemFailureExitCode() (int, error) {
	exitCodeRaw, ok := os.LookupEnv("SYSTEM_FAILURE_EXIT_CODE")
	if !ok || strings.TrimSpace(exitCodeRaw) == "" {
		return 2, nil
	}

	exitCode, err := strconv.Atoi(exitCodeRaw)
	if err != nil {
		return 0, fmt.Errorf("invalid SYSTEM_FAILURE_EXIT_CODE value: %v", err)
	}

	return exitCode, nil
}
