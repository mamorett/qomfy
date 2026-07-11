package commands

import (
	"fmt"

	"qomfy/internal/comfy"
	"qomfy/internal/overrides"
	"qomfy/internal/workflow"

	"github.com/spf13/cobra"
)

func init() {
	rigGLBCmd.Flags().StringVar(&rigWorkflowFile, "workflow-file", "rig-glb", "Workflow to use: short name or path")
	rigGLBCmd.Flags().StringVar(&rigMesh, "mesh", "", "Override Hy3DUploadMesh mesh input (path or ComfyUI reference)")
	rigGLBCmd.Flags().StringVar(&rigGLBName, "glb-name", "", "Output GLB base name (default: stem of input mesh)")
	rigGLBCmd.Flags().BoolVar(&rigNoFingers, "no-fingers", false, "Override MIAAutoRig no_fingers")
	rigGLBCmd.Flags().BoolVar(&rigUseNormal, "use-normal", false, "Override MIAAutoRig use_normal")
	rigGLBCmd.Flags().BoolVar(&rigResetToRest, "reset-to-rest", false, "Override MIAAutoRig reset_to_rest")
	RootCmd.AddCommand(rigGLBCmd)
}

var (
	rigWorkflowFile string
	rigMesh         string
	rigGLBName      string
	rigNoFingers    bool
	rigUseNormal    bool
	rigResetToRest  bool
)

var rigGLBCmd = &cobra.Command{
	Use:   "rig-glb",
	Short: "Auto-rig a GLB mesh using the MIA workflow and download the resulting GLB",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := resolveConfig(cmd)
		if err != nil {
			die("%s", err)
		}
		path, err := workflow.Resolve(rigWorkflowFile, resolveWorkflowsDir())
		if err != nil {
			die("%s", err)
		}
		prompt, err := loadPrompt(path)
		if err != nil {
			die("%s", err)
		}

		cl := clientFor(cfg)
		changes := []string{}

		resolvedMesh := rigMesh
		if rigMesh != "" {
			if isRegularFile(rigMesh) {
				ref := uploadAsset(cl, rigMesh, "Mesh")
				resolvedMesh = ref
				changes = append(changes, overrides.MeshUpload(rigMesh, ref))
			}
			c, err := overrides.Mesh(prompt, resolvedMesh)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}

		meshNodeID, meshNode, ok := overrides.FindNodeByClass(prompt, "Hy3DUploadMesh")
		if !ok {
			die("Could not find Hy3DUploadMesh node in workflow.")
		}
		_ = meshNodeID
		meshInputs := overrides.Inputs(meshNode)
		if meshInputs == nil {
			die("Could not determine Hy3DUploadMesh mesh input for default glb_name.")
		}
		meshInput, _ := meshInputs["mesh"].(string)
		if meshInput == "" {
			die("Could not determine Hy3DUploadMesh mesh input for default glb_name.")
		}

		resolvedGLBName := rigGLBName
		if resolvedGLBName == "" {
			resolvedGLBName = trimExt(meshInput)
		}
		if resolvedGLBName == "" {
			die("Derived glb_name is empty.")
		}
		c, err := overrides.FBXName(prompt, resolvedGLBName)
		if err != nil {
			die("%s", err)
		}
		changes = append(changes, c...)

		if cmd.Flags().Changed("no-fingers") {
			c, err := overrides.NoFingers(prompt, rigNoFingers)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		if cmd.Flags().Changed("use-normal") {
			c, err := overrides.UseNormal(prompt, rigUseNormal)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}
		if cmd.Flags().Changed("reset-to-rest") {
			c, err := overrides.ResetToRest(prompt, rigResetToRest)
			if err != nil {
				die("%s", err)
			}
			changes = append(changes, c...)
		}

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
