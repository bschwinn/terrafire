package main

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(groupsCmd)
	RootCmd.AddCommand(planCmd)
	RootCmd.AddCommand(applyCmd)
	RootCmd.AddCommand(destroyCmd)
	RootCmd.AddCommand(infoCmd)
}

// sub-commands
var RootCmd = &cobra.Command{
	Use:   "terrafire",
	Short: "Terrafire is an AWS provisioner.",
	Long:  "Terrafire is an AWS provisioner.",
	Run: func(cmd *cobra.Command, args []string) {
		if ourConfig.Debug {
			cmd.DebugFlags()
		} else {
			cmd.Help()
		}
	},
}

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Show all groups.",
	Long:  `This will show all groups currently configured.`,
	RunE:  runGroups,
}

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show the plan for a group.",
	Long:  `This will show what resources will be created in a group (group name required).`,
	RunE:  runPlan,
}

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show the existing resources for a group.",
	Long:  `This will show what resources currently are live for a group (group name required).`,
	RunE:  runInfo,
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Run the plan for a group.",
	Long:  `This will create all resources for a given group (group name required).`,
	RunE:  runApply,
}

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Run the destroy plan for a group.",
	Long:  `This will DESTROY all resources for a given group (group name required).`,
	RunE:  runDestroy,
}
