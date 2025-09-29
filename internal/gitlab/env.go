package gitlab

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	EnvironmentPrefix = "CUSTOM_ENV_"
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
	URL               string
	Token             string
	ProjectID         string
	MachineImage      string
	MachineType       string
	Region            string
	SSHPrivateKeyPath string
	JobID             string
	JobURL            string
	Debug             bool
}

func lookupEnv(key string) (string, bool) {
	return os.LookupEnv(fmt.Sprintf("%s%s", EnvironmentPrefix, key))
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

	env.MachineImage, _ = lookupEnv("GETMAC_CLOUD_MACHINE_IMAGE")
	if env.MachineImage == "" {
		env.MachineImage = "macos-sequoia"
	}

	env.MachineType, _ = lookupEnv("GETMAC_CLOUD_MACHINE_TYPE")
	if env.MachineType == "" {
		env.MachineType = "mac-m4-c4-m8"
	}

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
