package commands

import (
	"fmt"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

func init() {
	imageToGLBCmd.Flags().StringVar(&i2gImage, "image", "", "Local input image path (required)")
	imageToGLBCmd.Flags().StringVar(&i2gWorkflowFile, "workflow-file", "image-to-glb", "Workflow to use: short name or path")
	imageToGLBCmd.Flags().IntVar(&i2gMeshSeed, "mesh-seed", 0, "Override Trellis mesh seed")
	imageToGLBCmd.Flags().IntVar(&i2gTargetFaceNum, "target-face-num", 0, "Override target face count")
	imageToGLBCmd.Flags().StringVar(&i2gFilenamePrefix, "filename-prefix", "", "Override output filename prefix")
	imageToGLBCmd.Flags().IntVar(&i2gTextureSeed, "texture-seed", 0, "Override Trellis texture seed")
	_ = imageToGLBCmd.MarkFlagRequired("image")
	RootCmd.AddCommand(imageToGLBCmd)
}

var (
	i2gImage          string
	i2gWorkflowFile   string
	i2gMeshSeed       int
	i2gTargetFaceNum  int
	i2gFilenamePrefix string
	i2gTextureSeed    int
)

var imageToGLBCmd = &cobra.Command{
	Use:   "image-to-glb",
	Short: "Convert image to GLB using img_to_trellis2 workflow",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		path, err := workflow.Resolve(i2gWorkflowFile, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		prompt, err := loadPrompt(path)
		if err != nil {
			die("%s", err)
		}

		cl := clientFor(cfg)
		change := uploadImage(cl, prompt, i2gImage)

		changes, err := overrides.Apply(
			prompt, "",
			i2gMeshSeed, i2gTargetFaceNum, i2gTextureSeed,
			cmd.Flags().Changed("mesh-seed"),
			cmd.Flags().Changed("target-face-num"),
			cmd.Flags().Changed("texture-seed"),
			i2gFilenamePrefix,
		)
		if err != nil {
			die("%s", err)
		}
		changes = append(changes, change)
		echoChanges(changes)

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
