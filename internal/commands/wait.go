package commands

import (
	"fmt"

	"qomfy/internal/comfy"

	"github.com/spf13/cobra"
)

func init() {
	waitCmd.Flags().BoolVar(&waitDownloadGLB, "download-glb", true, "Download generated GLB when available")
	RootCmd.AddCommand(waitCmd)
}

var waitDownloadGLB bool

var waitCmd = &cobra.Command{
	Use:   "wait <prompt-id>",
	Short: "Poll queue/history until prompt completes; optionally download GLB output",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		promptID := args[0]
		cl := clientFor(cfg)

		queueState, historyItem, err := comfy.WaitForCompletion(
			cl, promptID, clientID(), cfg.PollInterval, cfg.Timeout, flagVerbose, nil)
		if err != nil {
			die("%s", err)
		}
		fmt.Println("Prompt completed.")
		printJSON(map[string]interface{}{"prompt_id": promptID, "queue": queueState})

		if waitDownloadGLB {
			downloaded, err := downloadWithProgress(cl, promptID, historyItem, cfg.DownloadsDir, comfy.GLBExtensions)
			if err != nil {
				die("%s", err)
			}
			if len(downloaded) == 0 {
				fmt.Println("No GLB reference found in history outputs.")
				return
			}
			for _, p := range downloaded {
				fmt.Printf("Downloaded %s\n", p)
			}
		}
	},
}
