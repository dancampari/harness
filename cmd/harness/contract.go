package harness

import (
	"fmt"
	"strings"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/events"
	"github.com/spf13/cobra"
)

func newContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contract",
		Short: "Manage deterministic multi-agent contract agreement",
	}
	cmd.AddCommand(
		newContractStatusCmd(),
		newContractProposeCmd(),
		newContractApproveCmd(),
		newContractRejectCmd(),
	)
	return cmd
}

func newContractStatusCmd() *cobra.Command {
	var sprintNumber int
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show agreement state for the current sprint contract",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := agreement.NewManager(".harness")
			st, err := mgr.Status(sprintNumber)
			if err != nil {
				return err
			}
			printAgreementStatus(st)
			return nil
		},
	}
	cmd.Flags().IntVar(&sprintNumber, "sprint", 0, "sprint number, defaults to current sprint")
	return cmd
}

func newContractProposeCmd() *cobra.Command {
	var sprintNumber int
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose the current sprint contract hash for agent agreement",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := agreement.NewManager(".harness")
			st, err := mgr.Propose(sprintNumber)
			if err != nil {
				return err
			}
			events.Record(".harness", "contract.proposed", events.PhaseContract,
				fmt.Sprintf("sprint %03d", st.SprintNumber), "")
			printAgreementStatus(st)
			return nil
		},
	}
	cmd.Flags().IntVar(&sprintNumber, "sprint", 0, "sprint number, defaults to current sprint")
	return cmd
}

func newContractApproveCmd() *cobra.Command {
	var sprintNumber int
	var role string
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve the current contract hash for one required agent role",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := agreement.NewManager(".harness")
			st, err := mgr.Approve(sprintNumber, role)
			if err != nil {
				return err
			}
			events.Record(".harness", "contract.approved", events.PhaseContract,
				fmt.Sprintf("%s · sprint %03d", role, st.SprintNumber), "")
			if strings.EqualFold(st.State, "agreed") {
				events.Record(".harness", "contract.agreed", events.PhaseContract,
					fmt.Sprintf("sprint %03d", st.SprintNumber), "")
			}
			printAgreementStatus(st)
			return nil
		},
	}
	cmd.Flags().IntVar(&sprintNumber, "sprint", 0, "sprint number, defaults to current sprint")
	cmd.Flags().StringVar(&role, "role", "", "required role approving this contract: planner|tester")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

func newContractRejectCmd() *cobra.Command {
	var sprintNumber int
	var role string
	var reason string
	cmd := &cobra.Command{
		Use:   "reject",
		Short: "Reject the current contract hash for one required agent role",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := agreement.NewManager(".harness")
			st, err := mgr.Reject(sprintNumber, role, reason)
			if err != nil {
				return err
			}
			events.Record(".harness", "contract.rejected", events.PhaseContract,
				fmt.Sprintf("%s · sprint %03d · %s", role, st.SprintNumber, reason), "")
			printAgreementStatus(st)
			return nil
		},
	}
	cmd.Flags().IntVar(&sprintNumber, "sprint", 0, "sprint number, defaults to current sprint")
	cmd.Flags().StringVar(&role, "role", "", "required role rejecting this contract: planner|tester")
	cmd.Flags().StringVar(&reason, "reason", "", "why this contract is not acceptable")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func printAgreementStatus(st agreement.Status) {
	shortHash := st.ContractHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	fmt.Printf("Contract sprint %03d  state=%s  hash=%s\n",
		st.SprintNumber, strings.ToUpper(st.State), valueOr(shortHash, "-"))
	if len(st.RequiredRoles) > 0 {
		fmt.Printf("  Required: %s\n", strings.Join(st.RequiredRoles, ", "))
	}
	if len(st.ApprovedRoles) > 0 {
		fmt.Printf("  Approved: %s\n", strings.Join(st.ApprovedRoles, ", "))
	}
	if len(st.RejectedRoles) > 0 {
		fmt.Printf("  Rejected: %s\n", strings.Join(st.RejectedRoles, ", "))
	}
	if len(st.MissingRoles) > 0 {
		fmt.Printf("  Missing:  %s\n", strings.Join(st.MissingRoles, ", "))
	}
	if len(st.Hashed) > 0 {
		fmt.Printf("  Hashed:   %s\n", strings.Join(st.Hashed, ", "))
	}
	if st.Reason != "" {
		fmt.Printf("  Reason:   %s\n", st.Reason)
	}
}
