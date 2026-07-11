package commands

import (
	"fmt"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

func init() {
	textToImageCmd.Flags().StringVar(&ttiPrompt, "prompt", "", "Text prompt to generate the image (required)")
	textToImageCmd.Flags().StringVar(&ttiWorkflowFile, "workflow-file", "text-to-image", "Workflow to use: short name or path")
	textToImageCmd.Flags().IntVar(&ttiSeed, "seed", 0, "Override KSampler seed")
	textToImageCmd.Flags().StringVar(&ttiFilenamePrefix, "filename-prefix", "", "Override SaveImage filename prefix")
	_ = textToImageCmd.MarkFlagRequired("prompt")
	RootCmd.AddCommand(textToImageCmd)
}

var (
	ttiPrompt         string
	ttiWorkflowFile   string
	ttiSeed           int
	ttiFilenamePrefix string
)

var textToImageCmd = &cobra.Command{
	Use:   "text-to-image",
	Short: "Generate image from text using qwen_image_2512 workflow",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		path, err := workflow.Resolve(ttiWorkflowFile, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		prompt, err := loadPrompt(path)
		if err != nil {
			die("%s", err)
		}

		changes, err := overrides.Apply(
			prompt, ttiPrompt, 0, 0, 0, false, false, false, "",
		)
		if err != nil {
			die("%s", err)
		}
		if cmd.Flags().Changed("seed") {
			c, err := overrides.Seed(prompt, ttiSeed)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		if ttiFilenamePrefix != "" {
			c, err := overrides.SaveImagePrefix(prompt, ttiFilenamePrefix)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		echoChanges(changes)

		cl := clientFor(cfg)
		downloaded, err := submitWaitDownload(cl, prompt, clientID(), cfg.PollInterval, cfg.Timeout, cfg.DownloadsDir, comfy.ImageExtensions, flagVerbose)
		if err != nil {
			die("%s", err)
		}
		if len(downloaded) == 0 {
			fmt.Println("No image output reference found in history outputs.")
			return
		}
		for _, p := range downloaded {
			fmt.Printf("Downloaded %s\n", p)
		}
	},
}
