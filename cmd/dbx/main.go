package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string
	var verbose bool
	var noCleanup bool

	rootCmd := &cobra.Command{
		Use:           "dbx",
		Short:         "Manage AWS SSM port-forwarding sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noCleanup, "no-cleanup", false, "Skip stopping sessions on exit")

	rootCmd.AddCommand(newConnectCmd())
	rootCmd.AddCommand(newLsCmd())
	rootCmd.AddCommand(newLogsCmd())
	rootCmd.AddCommand(newStopCmd())

	_ = configPath
	_ = verbose
	_ = noCleanup

	return rootCmd
}

func newConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <service> <env>",
		Short: "Start a port-forward session",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		},
	}
}

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List running sessions",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		},
	}
}

func newLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <service>/<env>",
		Short: "Show session logs",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <service>/<env>",
		Short: "Stop session(s)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		},
	}
}
