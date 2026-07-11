package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(healthCmd)
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check server connectivity via /system_stats",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		cl := clientFor(cfg)
		payload, err := cl.Health()
		if err != nil {
			die("%s", err)
		}
		fmt.Println("Connected to ComfyUI")
		printJSON(payload)
	},
}
