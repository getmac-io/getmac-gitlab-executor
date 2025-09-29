package command

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-sdk-golang"
	"github.com/spf13/cobra"
)

func NewPrepareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare command to create a virtual machine for the job",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runPrepareCommand(cmd, args)
			if err != nil {
				return gitlab.NewSystemFailureError(err)
			}

			return nil
		},
		Args: cobra.NoArgs,
	}

	return cmd
}

func runPrepareCommand(cmd *cobra.Command, args []string) error {
	env, err := gitlab.NewEnvironment()
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	slog.Info("Creating virtual machine...")

	client := getmac.NewClient(
		getmac.WithToken(env.Token), getmac.WithBaseURL(env.URL))

	_, vm, err := client.VirtualMachines().Create(cmd.Context(), env.ProjectID, &getmac.CreateVirtualMachineRequest{
		Name:   fmt.Sprintf("gitlab-job-%s", env.JobID),
		Image:  env.MachineImage,
		Type:   env.MachineType,
		Region: env.Region,
	})
	if err != nil {
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}

	slog.Info("Virtual machine created", "id", vm.ID)
	slog.Info("Waiting 30s for the virtual machine to boot up...")
	time.Sleep(30 * time.Second)

	return nil
}
