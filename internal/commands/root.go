// Package commands implements the qomfy CLI commands, mirroring cc.py 1:1:
// same command names, flags, defaults, override strings, and error messages.
package commands

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"qomfy/internal/comfy"
	qconfig "qomfy/internal/config"
	"qomfy/internal/overrides"
	"qomfy/internal/progress"

	"github.com/spf13/cobra"
)

// Shared persistent flag values (set on the root command).
var (
	flagConfig       string
	flagClientID     string
	flagPollInterval float64
	flagTimeout      float64
	flagVerbose      bool
	flagWorkflowsDir string
	flagDownloadsDir string
)

// RootCmd is the top-level `qomfy` command.
var RootCmd = &cobra.Command{
	Use:   "qomfy",
	Short: "Submit ComfyUI workflows and download their outputs.",
	Long:  "qomfy submits ComfyUI API-prompt workflows to a server and downloads the generated outputs.",
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to config.json (lookup order: --config, $QOMFY_CONFIG, $XDG_CONFIG_HOME/qomfy/config.json, ~/.config/qomfy/config.json)")
	RootCmd.PersistentFlags().StringVar(&flagClientID, "client-id", "", "ComfyUI client_id (random UUID each run if omitted)")
	RootCmd.PersistentFlags().Float64Var(&flagPollInterval, "poll-interval", 0, "Polling interval in seconds (default 2.0 or config value)")
	RootCmd.PersistentFlags().Float64Var(&flagTimeout, "timeout", 0, "Max wait time in seconds (default 1800.0 or config value)")
	RootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Log polling progress")
	RootCmd.PersistentFlags().StringVar(&flagWorkflowsDir, "workflows-dir", "", "Override installed workflows directory")
	RootCmd.PersistentFlags().StringVar(&flagDownloadsDir, "downloads-dir", "", "Output directory for downloaded files (default 'downloads' or config value)")
}

// resolveConfig loads the config following the lookup order and applies
// flag-level overrides. It returns an error (matching cc.py) when the config is
// missing or server_url is empty.
func resolveConfig(cmd *cobra.Command) (*qconfig.Config, error) {
	path := qconfig.ResolvePath(flagConfig)
	cfg, err := qconfig.Load(path)
	if err != nil {
		return nil, err
	}
	if cmd != nil {
		if cmd.Flags().Changed("poll-interval") {
			cfg.PollInterval = flagPollInterval
		}
		if cmd.Flags().Changed("timeout") {
			cfg.Timeout = flagTimeout
		}
	}
	if flagWorkflowsDir != "" {
		cfg.WorkflowsDir = qconfig.ExpandHome(flagWorkflowsDir)
	}
	if flagDownloadsDir != "" {
		cfg.DownloadsDir = qconfig.ExpandHome(flagDownloadsDir)
	}
	return cfg, nil
}

// resolveWorkflowsDir returns the workflows directory to use, preferring the
// --workflows-dir flag, then the config file, else the default.
func resolveWorkflowsDir() string {
	if flagWorkflowsDir != "" {
		return qconfig.ExpandHome(flagWorkflowsDir)
	}
	return qconfig.WorkflowsDirFrom(flagConfig, qconfig.ResolvePath(flagConfig))
}

// timeoutFor converts the resolved config timeout (seconds) into a duration for
// the HTTP client. Long-running waits use the per-poll timeout inside
// WaitForCompletion; the HTTP client timeout only bounds individual requests.
func timeoutFor(cfg *qconfig.Config) time.Duration {
	// cc.py uses 60s for /prompt, 120s for downloads, 20s for health. We use a
	// generous per-request bound so large uploads/downloads are not cut off.
	return 10 * time.Minute
}

// clientFor builds a ComfyUI client from a resolved config.
func clientFor(cfg *qconfig.Config) *comfy.Client {
	return comfy.NewClient(cfg.ServerURL, timeoutFor(cfg))
}

// clientID resolves a client id, generating a random UUID if none was given.
func clientID() string {
	if flagClientID != "" {
		return flagClientID
	}
	return newClientID()
}

func newClientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// loadPrompt reads a prompt/workflow JSON file and returns the ComfyUI API
// prompt map, mirroring cc.py's _load_prompt_from_file / _extract_prompt_payload.
func loadPrompt(path string) (overrides.Prompt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Prompt file not found: %s", path)
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Invalid JSON in %s: %w", path, err)
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Prompt JSON root must be an object.")
	}
	if prompt, ok := m["prompt"].(map[string]interface{}); ok {
		return overrides.Prompt(prompt), nil
	}
	if _, hasNodes := m["nodes"]; hasNodes {
		if _, hasLinks := m["links"]; hasLinks {
			return nil, fmt.Errorf("This looks like a ComfyUI workflow export (nodes/links graph). " +
				"The /prompt route expects API prompt JSON. In ComfyUI, export/copy " +
				"the API prompt format, or provide a file with a top-level 'prompt' object.")
		}
	}
	return overrides.Prompt(m), nil
}

// printJSON prints v as indented JSON followed by a newline.
func printJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%v\n", v)
		return
	}
	fmt.Println(string(b))
}

// echoChanges prints the "Applied overrides:" block identically to cc.py.
func echoChanges(changes []string) {
	if len(changes) == 0 {
		return
	}
	fmt.Println("Applied overrides:")
	for _, c := range changes {
		fmt.Printf("- %s\n", c)
	}
}

// die prints an error message to stderr and exits non-zero (mirrors typer's
// BadParameter exit behavior).
func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// downloadWithProgress extracts refs matching exts from historyItem, downloads
// each into outDir, and returns the written paths. A progress bar tracks the
// count. Returns nil (no error) when there are no matching refs.
func downloadWithProgress(cl *comfy.Client, promptID string, historyItem map[string]interface{}, outDir string, exts map[string]bool) ([]string, error) {
	refs := comfy.ExtractFileRefs(historyItem, exts)
	if len(refs) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	bar := progress.NewProgressBar(len(refs), "Downloading outputs")
	var downloaded []string
	for _, ref := range refs {
		fn := filepath.Base(ref)
		if fn == "" || fn == "." {
			fn = promptID + firstExt(exts)
		}
		dest := filepath.Join(outDir, fn)
		if err := cl.DownloadRef(ref, dest); err != nil {
			bar.Finish()
			return downloaded, err
		}
		downloaded = append(downloaded, dest)
		bar.IncrementWithStatus(dest)
	}
	bar.Finish()
	return downloaded, nil
}

func firstExt(exts map[string]bool) string {
	for e := range exts {
		return e
	}
	return ".bin"
}

// submitWaitDownload submits a prompt, waits for completion, and downloads the
// refs matching exts. It mirrors cc.py's _submit_wait_and_download.
func submitWaitDownload(cl *comfy.Client, prompt overrides.Prompt, cid string, poll, timeout float64, outDir string, exts map[string]bool, verbose bool) ([]string, error) {
	result, err := cl.SubmitPrompt(prompt, cid)
	if err != nil {
		return nil, err
	}
	printJSON(result)
	promptID, _ := result["prompt_id"].(string)
	if promptID == "" {
		return nil, fmt.Errorf("Unexpected /prompt response: %s", mustJSONString(result))
	}

	queueState, historyItem, err := comfy.WaitForCompletion(cl, promptID, cid, poll, timeout, verbose, nil)
	if err != nil {
		return nil, err
	}

	fmt.Println("Prompt completed.")
	printJSON(map[string]interface{}{"prompt_id": promptID, "queue": queueState})

	return downloadWithProgress(cl, promptID, historyItem, outDir, exts)
}

func mustJSONString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// submitAndWait submits a prompt and waits for completion, returning the
// prompt_id, queue state, and history item. It mirrors the cc.py submit+wait
// sequence used by the multi-stage commands.
func submitAndWait(cl *comfy.Client, prompt overrides.Prompt, cid string, poll, timeout float64, verbose bool) (string, map[string]interface{}, map[string]interface{}, error) {
	result, err := cl.SubmitPrompt(prompt, cid)
	if err != nil {
		return "", nil, nil, err
	}
	printJSON(result)
	promptID, _ := result["prompt_id"].(string)
	if promptID == "" {
		return "", nil, nil, fmt.Errorf("Unexpected /prompt response: %s", mustJSONString(result))
	}
	queueState, historyItem, err := comfy.WaitForCompletion(cl, promptID, cid, poll, timeout, verbose, nil)
	if err != nil {
		return "", nil, nil, err
	}
	fmt.Println("Prompt completed.")
	printJSON(map[string]interface{}{"prompt_id": promptID, "queue": queueState})
	return promptID, queueState, historyItem, nil
}

// trimExt is a small helper to strip an extension (used for glb_name derivation).
func trimExt(p string) string {
	return strings.TrimSuffix(p, filepath.Ext(p))
}

// uploadImage uploads a local image and patches all LoadImage nodes, returning
// the change string (mirrors cc.py's image-text-to-image / image-to-glb). It
// dies on error.
func uploadImage(cl *comfy.Client, prompt overrides.Prompt, imagePath string) string {
	if imagePath == "" {
		die("image path is required")
	}
	ref, err := cl.UploadImage(imagePath, true)
	if err != nil {
		die("%s", err)
	}
	updated := overrides.ReplaceAllLoadImage(prompt, ref)
	if len(updated) == 0 {
		die("Could not find LoadImage nodes to patch uploaded image.")
	}
	return overrides.ImageRef(ref, updated)
}

// uploadAsset uploads an arbitrary local file (e.g. a GLB mesh) and returns the
// reference string.
func uploadAsset(cl *comfy.Client, filePath, label string) string {
	ref, err := cl.UploadImage(filePath, true)
	if err != nil {
		die("%s upload failed: %s", label, err)
	}
	return ref
}

// isRegularFile reports whether path exists and is a regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
