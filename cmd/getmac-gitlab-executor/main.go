package main

import (
	"errors"
	"log"
	"os"

	"github.com/getmac-io/getmac-gitlab-executor/internal/command"
	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-gitlab-executor/internal/version"
	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "getmac-gitlab-executor",
		Short:             "Custom GitLab Runner executor to run CI/CD jobs in ephemeral GetMac virtual machines",
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		SilenceErrors:     true,
		SilenceUsage:      true,
	}

	cmd.SetHelpCommand(&cobra.Command{Hidden: true})
	cmd.SetVersionTemplate(version.GetVersion())
	cmd.Version = version.Version

	cmd.AddCommand(command.NewConfigCommand())
	cmd.AddCommand(command.NewPrepareCommand())
	cmd.AddCommand(command.NewRunCommand())
	cmd.AddCommand(command.NewCleanupCommand())

	return cmd
}

func main() {
	rootCmd := newRootCommand()

	buildFailureExitCode, err := gitlab.GetBuildFailureExitCode()
	if err != nil {
		log.Fatalf("Invalid exit code for build failure (BUILD_FAILURE_EXIT_CODE): %v", err)
	}

	systemFailureExitCode, err := gitlab.GetSystemFailureExitCode()
	if err != nil {
		log.Fatalf("Invalid exit code for system failure (SYSTEM_FAILURE_EXIT_CODE): %v", err)
	}

	err = rootCmd.Execute()
	if err != nil {
		rootCmd.PrintErrln("Error:", err)

		var systemFailureErr *gitlab.SystemFailureError
		if errors.As(err, &systemFailureErr) {
			os.Exit(systemFailureExitCode)
		}

		os.Exit(buildFailureExitCode)
	}
}
