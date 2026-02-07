package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fredyranthun/db/internal/config"
	"github.com/fredyranthun/db/internal/session"
	"github.com/fredyranthun/db/internal/ui"
	"github.com/spf13/cobra"
)

const defaultLogLines = 100

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type app struct {
	configPath string
	verbose    bool
	noCleanup  bool

	manager appSessionManager
}

type appSessionManager interface {
	Start(opts session.StartOptions) (*session.Session, error)
	Stop(key session.SessionKey) error
	StopAll() error
	List() []session.SessionSummary
	Get(key session.SessionKey) (*session.Session, bool)
	LastLogs(key session.SessionKey, n int) ([]string, error)
	SubscribeLogs(key session.SessionKey, buffer int) (uint64, <-chan string, error)
	UnsubscribeLogs(key session.SessionKey, id uint64)
}

type teaRunner interface {
	Run() (tea.Model, error)
}

var newTeaRunner = func(model tea.Model, opts ...tea.ProgramOption) teaRunner {
	return tea.NewProgram(model, opts...)
}

func main() {
	a := &app{
		manager: session.NewManager(),
	}

	rootCmd := newRootCmd(a)
	stopSignalCleanup := a.installSignalCleanup(rootCmd.ErrOrStderr())
	defer stopSignalCleanup()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(a *app) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "dbx",
		Short:         "Manage AWS SSM port-forwarding sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildVersionString(),
	}

	rootCmd.PersistentFlags().StringVar(&a.configPath, "config", "", "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&a.verbose, "verbose", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&a.noCleanup, "no-cleanup", false, "Skip stopping sessions on exit")

	rootCmd.AddCommand(a.newConnectCmd())
	rootCmd.AddCommand(a.newLsCmd())
	rootCmd.AddCommand(a.newLogsCmd())
	rootCmd.AddCommand(a.newStopCmd())
	rootCmd.AddCommand(a.newUICmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildVersionString())
		},
	}
}

func buildVersionString() string {
	return fmt.Sprintf("%s (commit=%s date=%s)", version, commit, date)
}

func (a *app) newUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Launch terminal UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cfgPath, err := config.LoadConfig(a.configPath)
			if err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if a.verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "using config: %s\n", cfgPath)
			}

			if err := a.runUI(cfg); err != nil {
				return err
			}
			return a.cleanupSessions()
		},
	}
}

func (a *app) runUI(cfg *config.Config) error {
	if a.verbose {
		fmt.Fprintf(
			os.Stderr,
			"ui debug: stdin_tty=%t stdout_tty=%t term=%q\n",
			isTTY(os.Stdin),
			isTTY(os.Stdout),
			os.Getenv("TERM"),
		)
		fmt.Fprintln(os.Stderr, "ui debug: using github.com/charmbracelet/bubbletea runtime")
	}

	runner := newTeaRunner(ui.NewModel(a.manager, cfg))
	if a.verbose {
		fmt.Fprintln(os.Stderr, "ui debug: starting bubbletea run loop")
	}
	_, err := runner.Run()
	if a.verbose {
		fmt.Fprintf(os.Stderr, "ui debug: bubbletea exited err=%v\n", err)
	}
	return err
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (a *app) installSignalCleanup(errOut io.Writer) func() {
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var once sync.Once

	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-done:
			return
		case <-sigCh:
		}

		once.Do(func() {
			if err := a.cleanupSessions(); err != nil {
				fmt.Fprintf(errOut, "cleanup failed: %v\n", err)
			}
			os.Exit(130)
		})
	}()

	return func() {
		close(done)
		signal.Stop(sigCh)
	}
}

func (a *app) cleanupSessions() error {
	if a.noCleanup || a.manager == nil {
		return nil
	}
	if err := a.manager.StopAll(); err != nil {
		return err
	}
	return nil
}

func (a *app) newConnectCmd() *cobra.Command {
	var localPort int
	var bindOverride string
	var profileOverride string
	var regionOverride string

	cmd := &cobra.Command{
		Use:   "connect <service> <env>",
		Short: "Start a port-forward session",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := strings.TrimSpace(args[0])
			envName := strings.TrimSpace(args[1])
			if serviceName == "" || envName == "" {
				return fmt.Errorf("service and env are required")
			}

			cfg, cfgPath, err := config.LoadConfig(a.configPath)
			if err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if a.verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "using config: %s\n", cfgPath)
			}

			defaults := cfg.EffectiveDefaults()
			envCfg, err := findEnvConfig(cfg, serviceName, envName)
			if err != nil {
				return err
			}

			bind := defaults.Bind
			if bindOverride != "" {
				bind = bindOverride
			}
			profile := defaults.Profile
			if profileOverride != "" {
				profile = profileOverride
			}
			region := defaults.Region
			if regionOverride != "" {
				region = regionOverride
			}

			opts := session.StartOptions{
				Service:          serviceName,
				Env:              envName,
				Bind:             bind,
				PortMin:          defaults.PortRange[0],
				PortMax:          defaults.PortRange[1],
				TargetInstanceID: envCfg.TargetInstanceID,
				RemoteHost:       envCfg.RemoteHost,
				RemotePort:       envCfg.RemotePort,
				Region:           region,
				Profile:          profile,
				StartupTimeout:   time.Duration(defaults.StartupTimeoutSeconds) * time.Second,
			}
			if localPort > 0 {
				opts.LocalPort = localPort
			}

			s, err := a.manager.Start(opts)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "service=%s env=%s\n", s.Service, s.Env)
			fmt.Fprintf(cmd.OutOrStdout(), "remote=%s:%d\n", s.RemoteHost, s.RemotePort)
			fmt.Fprintf(cmd.OutOrStdout(), "ENDPOINT=%s:%d\n", s.Bind, s.LocalPort)
			return nil
		},
	}

	cmd.Flags().IntVar(&localPort, "port", 0, "Local bind port override")
	cmd.Flags().StringVar(&bindOverride, "bind", "", "Local bind address override")
	cmd.Flags().StringVar(&profileOverride, "profile", "", "AWS profile override")
	cmd.Flags().StringVar(&regionOverride, "region", "", "AWS region override")

	return cmd
}

func (a *app) newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List running sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			summaries := a.manager.List()
			if len(summaries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no sessions")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tENDPOINT\tSTATE\tUPTIME\tPID\tERROR")
			for _, summary := range summaries {
				fmt.Fprintf(
					w,
					"%s\t%s:%d\t%s\t%s\t%d\t%s\n",
					summary.Key,
					summary.Bind,
					summary.LocalPort,
					summary.State,
					formatUptime(summary.Uptime),
					summary.PID,
					summary.LastError,
				)
			}
			return w.Flush()
		},
	}
}

func (a *app) newLogsCmd() *cobra.Command {
	var follow bool
	var lines int

	cmd := &cobra.Command{
		Use:   "logs <service>/<env>",
		Short: "Show session logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lines < 0 {
				return fmt.Errorf("lines must be >= 0")
			}

			serviceName, envName, err := parseServiceEnvPair(args[0])
			if err != nil {
				return err
			}
			key := session.NewSessionKey(serviceName, envName)

			s, ok := a.manager.Get(key)
			if !ok || s == nil {
				return fmt.Errorf("%s: session not found", key)
			}

			for _, line := range s.LastLogs(lines) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if !follow {
				return nil
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			lastPrinted := len(s.LastLogs(session.DefaultRingBufferLines))
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					current, ok := a.manager.Get(key)
					if !ok || current == nil {
						return nil
					}
					all := current.LastLogs(session.DefaultRingBufferLines)
					if lastPrinted > len(all) {
						lastPrinted = len(all)
					}
					for _, line := range all[lastPrinted:] {
						fmt.Fprintln(cmd.OutOrStdout(), line)
					}
					lastPrinted = len(all)
				case <-sigCh:
					return nil
				}
			}
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&lines, "lines", defaultLogLines, "Number of lines to show from the end")

	return cmd
}

func (a *app) newStopCmd() *cobra.Command {
	var stopAll bool

	cmd := &cobra.Command{
		Use:   "stop <service>/<env> | <service> <env> | --all",
		Short: "Stop session(s)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stopAll {
				if len(args) > 0 {
					return fmt.Errorf("--all does not accept positional args")
				}
				if err := a.manager.StopAll(); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "stopped all sessions")
				return nil
			}

			serviceName, envName, err := parseStopArgs(args)
			if err != nil {
				return err
			}

			key := session.NewSessionKey(serviceName, envName)
			if err := a.manager.Stop(key); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stopped %s\n", key)
			return nil
		},
	}

	cmd.Flags().BoolVar(&stopAll, "all", false, "Stop all sessions")

	return cmd
}

func findEnvConfig(cfg *config.Config, serviceName, envName string) (config.EnvConfig, error) {
	for _, svc := range cfg.Services {
		if svc.Name != serviceName {
			continue
		}

		envCfg, ok := svc.Envs[envName]
		if !ok {
			return config.EnvConfig{}, fmt.Errorf("%s/%s: environment not found in config", serviceName, envName)
		}
		return envCfg, nil
	}
	return config.EnvConfig{}, fmt.Errorf("%s/%s: service not found in config", serviceName, envName)
}

func parseStopArgs(args []string) (string, string, error) {
	switch len(args) {
	case 1:
		return parseServiceEnvPair(args[0])
	case 2:
		serviceName := strings.TrimSpace(args[0])
		envName := strings.TrimSpace(args[1])
		if serviceName == "" || envName == "" {
			return "", "", fmt.Errorf("service and env are required")
		}
		return serviceName, envName, nil
	default:
		return "", "", fmt.Errorf("usage: dbx stop <service>/<env> | <service> <env> | --all")
	}
}

func parseServiceEnvPair(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected <service>/<env>, got %q", value)
	}

	serviceName := strings.TrimSpace(parts[0])
	envName := strings.TrimSpace(parts[1])
	if serviceName == "" || envName == "" {
		return "", "", fmt.Errorf("expected non-empty <service>/<env>, got %q", value)
	}

	return serviceName, envName, nil
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.Truncate(time.Second).String()
}
