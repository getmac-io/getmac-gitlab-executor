package command

import (
	"fmt"

	"github.com/getmac-io/getmac-gitlab-executor/internal/gitlab"
	"github.com/getmac-io/getmac-sdk-golang"
	"github.com/spf13/cobra"
)

func NewCleanupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup command to remove the virtual machine after job completion",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runCleanupCommand(cmd, args)
			if err != nil {
				return gitlab.NewSystemFailureError(err)
			}

			return nil
		},
		Args: cobra.NoArgs,
	}

	return cmd
}

func runCleanupCommand(cmd *cobra.Command, _ []string) error {
	env, err := gitlab.NewEnvironment()
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	client := getmac.NewClient(
		getmac.WithToken(env.Token), getmac.WithBaseURL(env.URL))

	_, vm, err := client.VirtualMachines().GetByName(cmd.Context(), env.ProjectID, fmt.Sprintf("gitlab-job-%s", env.JobID))
	if err != nil {
		return fmt.Errorf("failed to get virtual machine by name: %w", err)
	}

	if vm == nil {
		return nil
	}

	_, err = client.VirtualMachines().Delete(cmd.Context(), env.ProjectID, vm.ID)
	if err != nil {
		return fmt.Errorf("failed to delete virtual machine: %w", err)
	}

	return nil
}
