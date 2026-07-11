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
	textToRiggedGLBCmd.Flags().StringVar(&t2rgPrompt, "prompt", "", "Text prompt to generate the source image (required)")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgTextWorkflow, "text-workflow-file", "text-to-image", "Path to text-to-image API prompt JSON")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgGLBWorkflow, "glb-workflow-file", "image-to-glb", "Path to image-to-glb API prompt JSON")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgRigWorkflow, "rig-workflow-file", "rig-glb", "Path to rig_glb_mia API prompt JSON")
	t2rgRegisterFlags()
	RootCmd.AddCommand(textToRiggedGLBCmd)
}

func t2rgRegisterFlags() {
	textToRiggedGLBCmd.Flags().IntVar(&t2rgSeed, "seed", 0, "Override KSampler seed for text stage")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgImageFilenamePrefix, "image-filename-prefix", "", "Override SaveImage filename prefix for text stage")
	textToRiggedGLBCmd.Flags().IntVar(&t2rgMeshSeed, "mesh-seed", 0, "Override Trellis mesh seed")
	textToRiggedGLBCmd.Flags().IntVar(&t2rgTargetFaceNum, "target-face-num", 0, "Override target face count")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgFilenamePrefix, "filename-prefix", "", "Override Trellis export filename prefix")
	textToRiggedGLBCmd.Flags().IntVar(&t2rgTextureSeed, "texture-seed", 0, "Override Trellis texture seed")
	textToRiggedGLBCmd.Flags().StringVar(&t2rgGLBName, "glb-name", "", "Rigged GLB base name; defaults to the stem of the generated GLB")
	textToRiggedGLBCmd.Flags().BoolVar(&t2rgNoFingers, "no-fingers", false, "Override MIAAutoRig no_fingers")
	textToRiggedGLBCmd.Flags().BoolVar(&t2rgUseNormal, "use-normal", false, "Override MIAAutoRig use_normal")
	textToRiggedGLBCmd.Flags().BoolVar(&t2rgResetToRest, "reset-to-rest", false, "Override MIAAutoRig reset_to_rest")
}

var (
	t2rgPrompt              string
	t2rgTextWorkflow        string
	t2rgGLBWorkflow         string
	t2rgRigWorkflow         string
	t2rgSeed                int
	t2rgImageFilenamePrefix string
	t2rgMeshSeed            int
	t2rgTargetFaceNum       int
	t2rgFilenamePrefix      string
	t2rgTextureSeed         int
	t2rgGLBName             string
	t2rgNoFingers           bool
	t2rgUseNormal           bool
	t2rgResetToRest         bool
)

var textToRiggedGLBCmd = &cobra.Command{
	Use:   "text-to-rigged-glb",
	Short: "Generate and rig end-to-end: text-to-image, image-to-glb, then rig-glb",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		cl := clientFor(cfg)
		textWF, err := workflow.Resolve(t2rgTextWorkflow, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		glbWF, err := workflow.Resolve(t2rgGLBWorkflow, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		rigWF, err := workflow.Resolve(t2rgRigWorkflow, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}

		textOutDir := filepath.Join(cfg.DownloadsDir, "text_to_rigged_glb_images")
		glbOutDir := filepath.Join(cfg.DownloadsDir, "text_to_rigged_glb_glb")
		rigOutDir := filepath.Join(cfg.DownloadsDir, "text_to_rigged_glb_rigged")

		// --- Stage 1/3: text-to-image ---
		fmt.Println("Stage 1/3: text-to-image")
		textPrompt, err := loadPrompt(textWF)
		if err != nil {
			die("%s", err)
		}
		textChanges, err := overrides.Apply(textPrompt, t2rgPrompt, 0, 0, 0, false, false, false, "")
		if err != nil {
			die("%s", err)
		}
		if cmd.Flags().Changed("seed") {
			c, err := overrides.Seed(textPrompt, t2rgSeed)
			if err != nil {
				die("%s", err)
			}
			textChanges = append(textChanges, c...)
		}
		if t2rgImageFilenamePrefix != "" {
			c, err := overrides.ImagePrefixStage1(textPrompt, t2rgImageFilenamePrefix)
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

		// --- Stage 2/3: image-to-glb ---
		fmt.Println("Stage 2/3: image-to-glb")
		glbPrompt, err := loadPrompt(glbWF)
		if err != nil {
			die("%s", err)
		}
		glbChange := uploadImage(cl, glbPrompt, sourceImage)
		glbChanges, err := overrides.Apply(
			glbPrompt, "",
			t2rgMeshSeed, t2rgTargetFaceNum, t2rgTextureSeed,
			cmd.Flags().Changed("mesh-seed"),
			cmd.Flags().Changed("target-face-num"),
			cmd.Flags().Changed("texture-seed"),
			t2rgFilenamePrefix,
		)
		if err != nil {
			die("%s", err)
		}
		glbChanges = append(glbChanges, glbChange)
		echoChanges(glbChanges)

		stage2CID := clientID()
		stage2Result, err := cl.SubmitPrompt(glbPrompt, stage2CID)
		if err != nil {
			die("%s", err)
		}
		stage2PromptID, _ := stage2Result["prompt_id"].(string)
		if stage2PromptID == "" {
			die("Unexpected /prompt response for image-to-glb stage: %s", mustJSONString(stage2Result))
		}
		printJSON(stage2Result)
		queueState, historyItem, err := comfy.WaitForCompletion(cl, stage2PromptID, stage2CID, cfg.PollInterval, cfg.Timeout, flagVerbose, nil)
		if err != nil {
			die("%s", err)
		}
		fmt.Println("Image-to-GLB stage completed.")
		printJSON(map[string]interface{}{"prompt_id": stage2PromptID, "queue": queueState})

		glbRefs := comfy.ExtractFileRefs(historyItem, comfy.GLBExtensions)
		if len(glbRefs) == 0 {
			die("No GLB reference found in image-to-glb stage outputs.")
		}
		sourceGLBRef := glbRefs[0]
		fmt.Printf("Using GLB for rig stage: %s\n", sourceGLBRef)

		stage2Downloads, err := downloadWithProgress(cl, stage2PromptID, historyItem, glbOutDir, comfy.GLBExtensions)
		if err != nil {
			die("%s", err)
		}
		if len(stage2Downloads) == 0 {
			die("Failed to download GLB artifact for rig stage.")
		}
		downloadedGLB := stage2Downloads[0]
		uploadedGLBRef := uploadAsset(cl, downloadedGLB, "Mesh")
		for _, p := range stage2Downloads {
			fmt.Printf("Downloaded %s\n", p)
		}
		fmt.Printf("Uploaded GLB for rig stage: %s\n", uploadedGLBRef)

		// --- Stage 3/3: rig-glb ---
		fmt.Println("Stage 3/3: rig-glb")
		rigPrompt, err := loadPrompt(rigWF)
		if err != nil {
			die("%s", err)
		}
		rigChanges := []string{}

		c, err := overrides.Mesh(rigPrompt, uploadedGLBRef)
		if err != nil {
			die("%s", err)
		}
		rigChanges = append(rigChanges, c...)

		resolvedGLBName := t2rgGLBName
		if resolvedGLBName == "" {
			resolvedGLBName = trimExt(downloadedGLB)
		}
		if resolvedGLBName == "" {
			die("Derived glb_name is empty for rig stage.")
		}
		c, err = overrides.FBXName(rigPrompt, resolvedGLBName)
		if err != nil {
			die("%s", err)
		}
		rigChanges = append(rigChanges, c...)

		if cmd.Flags().Changed("no-fingers") {
			c, err := overrides.NoFingers(rigPrompt, t2rgNoFingers)
			if err != nil {
				die("%s", err)
			}
			rigChanges = append(rigChanges, c...)
		}
		if cmd.Flags().Changed("use-normal") {
			c, err := overrides.UseNormal(rigPrompt, t2rgUseNormal)
			if err != nil {
				die("%s", err)
			}
			rigChanges = append(rigChanges, c...)
		}
		if cmd.Flags().Changed("reset-to-rest") {
			c, err := overrides.ResetToRest(rigPrompt, t2rgResetToRest)
			if err != nil {
				die("%s", err)
			}
			rigChanges = append(rigChanges, c...)
		}

		echoChanges(rigChanges)

		riggedDownloads, err := submitWaitDownload(cl, rigPrompt, clientID(), cfg.PollInterval, cfg.Timeout, rigOutDir, comfy.GLBExtensions, flagVerbose)
		if err != nil {
			die("%s", err)
		}
		if len(riggedDownloads) == 0 {
			fmt.Println("No GLB reference found in rig stage history outputs.")
			return
		}
		for _, p := range riggedDownloads {
			fmt.Printf("Downloaded %s\n", p)
		}
	},
}
