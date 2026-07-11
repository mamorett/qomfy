package commands

import (
	"fmt"
	"os"
	"path/filepath"

	qconfig "qomfy/internal/config"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

var (
	configInitServerURL string
	configInitOut       string
	configInitForce     bool
)

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configWorkflowsCmd)

	configInitCmd.Flags().StringVar(&configInitServerURL, "server-url", qconfig.DefaultServerURL, "ComfyUI server URL")
	configInitCmd.Flags().StringVar(&configInitOut, "out", "", "Output config path (default: resolved config path)")
	configInitCmd.Flags().BoolVar(&configInitForce, "force", false, "Overwrite existing config")
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage local config",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create config.json for this project",
	Run: func(cmd *cobra.Command, args []string) {
		out := configInitOut
		if out == "" {
			out = qconfig.ResolvePath(flagConfig)
		}
		out = qconfig.ExpandHome(out)
		if !configInitForce {
			if _, err := filepath.Abs(out); err == nil {
				if _, statErr := os.Stat(out); statErr == nil {
					die("%s already exists. Pass --force to overwrite.", out)
				}
			}
		}
		cfg := qconfig.NewForInit(configInitServerURL)
		if err := cfg.Write(out); err != nil {
			die("Could not write config: %s", err)
		}
		fmt.Printf("Wrote %s\n", out)
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the resolved config file path",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(qconfig.ResolvePath(flagConfig))
	},
}

var configWorkflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Print resolved workflows directory and installed workflow short names",
	Run: func(cmd *cobra.Command, args []string) {
		dir := resolveWorkflowsDir()
		fmt.Printf("Workflows directory: %s\n", dir)
		for _, entry := range workflow.Installed(dir) {
			fmt.Printf("  %s -> %s\n", entry.ShortName, entry.Path)
		}
	},
}
