package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

// ShortName maps a qomfy short name to the installed workflow filename (without
// directory). These match qomfyplan.md.
var ShortName = map[string]string{
	"text-to-image":       "qwen_image_2512.json",
	"image-text-to-image": "qwen_image_edit_2511.json",
	"image-to-glb":        "img_to_trellis2.json",
	"rig-glb":             "rig_glb_mia.json",
}

// Reverse maps an installed filename back to its short name.
var fileToShort = func() map[string]string {
	m := make(map[string]string, len(ShortName))
	for short, file := range ShortName {
		m[file] = short
	}
	return m
}()

// Resolve expands a workflow reference into a usable file path.
//
//   - If ref is one of the known short names, it resolves to
//     <workflowsDir>/<file>.
//   - If ref is a fully-qualified filesystem path (exists), it is used verbatim.
//   - Otherwise ref is treated as a path relative to workflowsDir.
func Resolve(ref, workflowsDir string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("workflow reference is empty")
	}
	if file, ok := ShortName[ref]; ok {
		return filepath.Join(workflowsDir, file), nil
	}
	if filepath.IsAbs(ref) {
		if _, err := os.Stat(ref); err != nil {
			return "", fmt.Errorf("workflow file not found: %s", ref)
		}
		return ref, nil
	}
	// Treat as a relative path; check in CWD first, else workflowsDir.
	if _, err := os.Stat(ref); err == nil {
		abs, err := filepath.Abs(ref)
		if err == nil {
			return abs, nil
		}
		return ref, nil
	}
	candidate := filepath.Join(workflowsDir, ref)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("workflow %q not found in %s", ref, workflowsDir)
	}
	return candidate, nil
}

// Installed returns the short names that have an installed file in workflowsDir,
// paired with their resolved paths.
func Installed(workflowsDir string) []Entry {
	entries := make([]Entry, 0, len(ShortName))
	for short, file := range ShortName {
		entries = append(entries, Entry{
			ShortName: short,
			Path:      filepath.Join(workflowsDir, file),
		})
	}
	return entries
}

// Entry pairs a short name with its resolved path.
type Entry struct {
	ShortName string
	Path      string
}
