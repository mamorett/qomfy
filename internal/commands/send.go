package commands

import (
	"encoding/json"
	"fmt"

	"qomfy/internal/overrides"

	"github.com/spf13/cobra"
)

func init() {
	sendCmd.Flags().StringVar(&sendPrompt, "prompt", "", "Override positive prompt text")
	sendCmd.Flags().IntVar(&sendMeshSeed, "mesh-seed", 0, "Override Trellis mesh seed")
	sendCmd.Flags().IntVar(&sendTargetFaceNum, "target-face-num", 0, "Override target face count")
	sendCmd.Flags().StringVar(&sendFilenamePrefix, "filename-prefix", "", "Override output filename prefix")
	sendCmd.Flags().IntVar(&sendTextureSeed, "texture-seed", 0, "Override Trellis texture seed")
	sendCmd.Flags().BoolVar(&sendDryRun, "dry-run", false, "Build payload but do not POST")
	RootCmd.AddCommand(sendCmd)
}

var (
	sendPrompt         string
	sendMeshSeed       int
	sendTargetFaceNum  int
	sendFilenamePrefix string
	sendTextureSeed    int
	sendDryRun         bool
)

var sendCmd = &cobra.Command{
	Use:   "send <prompt-file>",
	Short: "Submit a prompt JSON file to ComfyUI /prompt",
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
			sendPrompt,
			sendMeshSeed, sendTargetFaceNum, sendTextureSeed,
			cmd.Flags().Changed("mesh-seed"),
			cmd.Flags().Changed("target-face-num"),
			cmd.Flags().Changed("texture-seed"),
			sendFilenamePrefix,
		)
		if err != nil {
			die("%s", err)
		}
		echoChanges(changes)

		payload := map[string]interface{}{
			"prompt":    prompt,
			"client_id": clientID(),
		}
		if sendDryRun {
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(b))
			return
		}

		cl := clientFor(cfg)
		result, err := cl.SubmitPrompt(prompt, clientID())
		if err != nil {
			die("%s", err)
		}
		printJSON(result)
	},
}
