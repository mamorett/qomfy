package commands

import (
	"fmt"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

func init() {
	imageTextToImageCmd.Flags().StringVar(&it2iImage, "image", "", "Local input image path (required)")
	imageTextToImageCmd.Flags().StringVar(&it2iPrompt, "prompt", "", "Edit prompt (required)")
	imageTextToImageCmd.Flags().StringVar(&it2iWorkflowFile, "workflow-file", "image-text-to-image", "Workflow to use: short name or path")
	imageTextToImageCmd.Flags().IntVar(&it2iSeed, "seed", 0, "Override KSampler seed")
	imageTextToImageCmd.Flags().StringVar(&it2iFilenamePrefix, "filename-prefix", "", "Override SaveImage filename prefix")
	_ = imageTextToImageCmd.MarkFlagRequired("image")
	_ = imageTextToImageCmd.MarkFlagRequired("prompt")
	RootCmd.AddCommand(imageTextToImageCmd)
}

var (
	it2iImage          string
	it2iPrompt         string
	it2iWorkflowFile   string
	it2iSeed           int
	it2iFilenamePrefix string
)

var imageTextToImageCmd = &cobra.Command{
	Use:   "image-text-to-image",
	Short: "Edit an image with text prompt using qwen_image_edit_2511 workflow",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		path, err := workflow.Resolve(it2iWorkflowFile, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		prompt, err := loadPrompt(path)
		if err != nil {
			die("%s", err)
		}

		cl := clientFor(cfg)
		change := uploadImage(cl, prompt, it2iImage)

		changes, err := overrides.Apply(
			prompt, it2iPrompt, 0, 0, 0, false, false, false, "",
		)
		if err != nil {
			die("%s", err)
		}
		changes = append(changes, change)
		if cmd.Flags().Changed("seed") {
			c, err := overrides.Seed(prompt, it2iSeed)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		if it2iFilenamePrefix != "" {
			c, err := overrides.SaveImagePrefix(prompt, it2iFilenamePrefix)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		echoChanges(changes)

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
