package command

import (
	"encoding/json"
	"fmt"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-gitlab-executor/internal/version"
	"github.com/spf13/cobra"
)

func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Config command to output the GitLab Runner configuration in JSON format",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runConfigCommand(cmd, args)
			if err != nil {
				return gitlab.NewSystemFailureError(err)
			}

			return nil
		},
		Args: cobra.NoArgs,
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().String("getmac-cloud-api-url", "https://api.getmac.io/v1", "Sets the GetMac Cloud API URL to use for subsequent commands")
	cmd.Flags().String("getmac-cloud-api-key", "", "Sets the GetMac Cloud API key to use for subsequent commands")
	cmd.MarkFlagRequired("getmac-cloud-api-key")
	cmd.Flags().String("builds-dir", "/tmp/builds", "Sets the builds directory")
	cmd.Flags().Bool("builds-dir-is-shared", false, "Sets whether the builds directory is shared")
	cmd.Flags().String("cache-dir", "/tmp/cache", "Sets the cache directory")

	return cmd
}

func runConfigCommand(cmd *cobra.Command, _ []string) error {
	apiUrl, err := cmd.Flags().GetString("getmac-cloud-api-url")
	if err != nil {
		return err
	}

	apiKey, err := cmd.Flags().GetString("getmac-cloud-api-key")
	if err != nil {
		return err
	}

	buildsDir, err := cmd.Flags().GetString("builds-dir")
	if err != nil {
		return err
	}

	buildsDirIsShared, err := cmd.Flags().GetBool("builds-dir-is-shared")
	if err != nil {
		return err
	}

	cacheDir, err := cmd.Flags().GetString("cache-dir")
	if err != nil {
		return err
	}

	config := gitlab.RunnerConfig{
		BuildsDir:         buildsDir,
		BuildsDirIsShared: buildsDirIsShared,
		CacheDir:          cacheDir,
		JobEnv: map[string]string{
			"CUSTOM_ENV_GETMAC_CLOUD_API_KEY": apiKey,
			"CUSTOM_ENV_GETMAC_CLOUD_API_URL": apiUrl,
		},
		Driver: gitlab.RunnerDriverConfig{
			Name:    "GetMac GitLab Executor",
			Version: version.Version,
		},
		Shell: "bash",
	}

	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonData))
	return nil
}
