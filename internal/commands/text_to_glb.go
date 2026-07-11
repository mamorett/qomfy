package commands

import (
	"fmt"
	"path/filepath"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

func init() {
	textToGLBCmd.Flags().StringVar(&t2gPrompt, "prompt", "", "Text prompt to generate the source image (required)")
	textToGLBCmd.Flags().StringVar(&t2gTextWorkflow, "text-workflow-file", "text-to-image", "Path to text-to-image API prompt JSON")
	textToGLBCmd.Flags().StringVar(&t2gGLBWorkflow, "glb-workflow-file", "image-to-glb", "Path to image-to-glb API prompt JSON")
	t2gCmdRegisterSeedFlags()
	RootCmd.AddCommand(textToGLBCmd)
}

func t2gCmdRegisterSeedFlags() {
	textToGLBCmd.Flags().IntVar(&t2gSeed, "seed", 0, "Override KSampler seed for text stage")
	textToGLBCmd.Flags().StringVar(&t2gImageFilenamePrefix, "image-filename-prefix", "", "Override SaveImage filename prefix for text stage")
	textToGLBCmd.Flags().IntVar(&t2gMeshSeed, "mesh-seed", 0, "Override Trellis mesh seed")
	textToGLBCmd.Flags().IntVar(&t2gTargetFaceNum, "target-face-num", 0, "Override target face count")
	textToGLBCmd.Flags().StringVar(&t2gFilenamePrefix, "filename-prefix", "", "Override Trellis export filename prefix")
	textToGLBCmd.Flags().IntVar(&t2gTextureSeed, "texture-seed", 0, "Override Trellis texture seed")
}

var (
	t2gPrompt              string
	t2gTextWorkflow        string
	t2gGLBWorkflow         string
	t2gSeed                int
	t2gImageFilenamePrefix string
	t2gMeshSeed            int
	t2gTargetFaceNum       int
	t2gFilenamePrefix      string
	t2gTextureSeed         int
)

var textToGLBCmd = &cobra.Command{
	Use:   "text-to-glb",
	Short: "Generate a GLB end-to-end: text-to-image, then image-to-glb",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		cl := clientFor(cfg)
		textWF, err := workflow.Resolve(t2gTextWorkflow, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		glbWF, err := workflow.Resolve(t2gGLBWorkflow, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}

		textOutDir := filepath.Join(cfg.DownloadsDir, "text_to_glb_images")
		glbOutDir := filepath.Join(cfg.DownloadsDir, "text_to_glb_glb")

		fmt.Println("Stage 1/2: text-to-image")
		textPrompt, err := loadPrompt(textWF)
		if err != nil {
			die("%s", err)
		}
		textChanges, err := overrides.Apply(textPrompt, t2gPrompt, 0, 0, 0, false, false, false, "")
		if err != nil {
			die("%s", err)
		}
		if cmd.Flags().Changed("seed") {
			c, err := overrides.Seed(textPrompt, t2gSeed)
			if err != nil {
				die("%s", err)
			}
			textChanges = append(textChanges, c...)
		}
		if t2gImageFilenamePrefix != "" {
			c, err := overrides.ImagePrefixStage1(textPrompt, t2gImageFilenamePrefix)
			if err != nil {
				die("%s", err)
			}
			textChanges = append(textChanges, c...)
		}
		echoChanges(textChanges)

		images, err := submitWaitDownload(cl, textPrompt, clientID(), cfg.PollInterval, cfg.Timeout, textOutDir, comfy.ImageExtensions, flagVerbose)
		if err != nil {
			die("%s", err)
		}
		if len(images) == 0 {
			die("No image output reference found in text-to-image stage.")
		}
		sourceImage := images[0]
		fmt.Printf("Using image for GLB stage: %s\n", sourceImage)

		fmt.Println("Stage 2/2: image-to-glb")
		glbPrompt, err := loadPrompt(glbWF)
		if err != nil {
			die("%s", err)
		}
		change := uploadImage(cl, glbPrompt, sourceImage)
		glbChanges, err := overrides.Apply(
			glbPrompt, "",
			t2gMeshSeed, t2gTargetFaceNum, t2gTextureSeed,
			cmd.Flags().Changed("mesh-seed"),
			cmd.Flags().Changed("target-face-num"),
			cmd.Flags().Changed("texture-seed"),
			t2gFilenamePrefix,
		)
		if err != nil {
			die("%s", err)
		}
		glbChanges = append(glbChanges, change)
		echoChanges(glbChanges)

		glbDownloads, err := submitWaitDownload(cl, glbPrompt, clientID(), cfg.PollInterval, cfg.Timeout, glbOutDir, comfy.GLBExtensions, flagVerbose)
		if err != nil {
			die("%s", err)
		}
		if len(glbDownloads) == 0 {
			fmt.Println("No GLB reference found in history outputs.")
			return
		}
		for _, p := range glbDownloads {
			fmt.Printf("Downloaded %s\n", p)
		}
	},
}
