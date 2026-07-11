package commands

import (
	"fmt"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"

	"github.com/spf13/cobra"
)

func init() {
	runCmd.Flags().StringVar(&runPrompt, "prompt", "", "Override positive prompt text")
	runCmd.Flags().IntVar(&runMeshSeed, "mesh-seed", 0, "Override Trellis mesh seed")
	runCmd.Flags().IntVar(&runTargetFaceNum, "target-face-num", 0, "Override target face count")
	runCmd.Flags().StringVar(&runFilenamePrefix, "filename-prefix", "", "Override output filename prefix")
	runCmd.Flags().IntVar(&runTextureSeed, "texture-seed", 0, "Override Trellis texture seed")
	RootCmd.AddCommand(runCmd)
}

var (
	runPrompt         string
	runMeshSeed       int
	runTargetFaceNum  int
	runFilenamePrefix string
	runTextureSeed    int
)

var runCmd = &cobra.Command{
	Use:   "run <prompt-file>",
	Short: "Submit prompt, wait asynchronously, and download GLB outputs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		prompt, err := loadPrompt(args[0])
		if err != nil {
			die("%s", err)
		}

		changes, err := overrides.Apply(
			prompt,
			runPrompt,
			runMeshSeed, runTargetFaceNum, runTextureSeed,
			cmd.Flags().Changed("mesh-seed"),
			cmd.Flags().Changed("target-face-num"),
			cmd.Flags().Changed("texture-seed"),
			runFilenamePrefix,
		)
		if err != nil {
			die("%s", err)
		}
		echoChanges(changes)

		cl := clientFor(cfg)
		downloaded, err := submitWaitDownload(cl, prompt, clientID(), cfg.PollInterval, cfg.Timeout, cfg.DownloadsDir, comfy.GLBExtensions, flagVerbose)
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
	},
}
