// Package overrides applies field overrides to a ComfyUI API-prompt payload.
//
// A ComfyUI API prompt is represented as map[nodeID] -> node, where each node is
// a map with at least "class_type" (string) and "inputs" (map). This package
// mutates the "inputs" of matching nodes and reports human-readable change
// strings that match cc.py's output verbatim.
package overrides

import (
	"fmt"
	"strconv"
	"strings"
)

// Prompt is the ComfyUI API prompt: node_id -> node object.
type Prompt map[string]interface{}

// formatVal renders an override value the way cc.py's f-strings would: bools as
// "True"/"False" (Python str(bool)), everything else as %v.
func formatVal(v interface{}) string {
	switch b := v.(type) {
	case bool:
		if b {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(b)
	case int64:
		return strconv.FormatInt(b, 10)
	case float64:
		if b == float64(int64(b)) {
			return strconv.FormatInt(int64(b), 10)
		}
		return strconv.FormatFloat(b, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// node returns the node object and its id for the given key.
func node(prompt Prompt, id string) (map[string]interface{}, bool) {
	n, ok := prompt[id].(map[string]interface{})
	return n, ok
}

// FindNodeByClass returns the id and node object of the first node whose
// class_type matches, plus whether one was found.
func FindNodeByClass(prompt Prompt, classType string) (string, map[string]interface{}, bool) {
	for id, raw := range prompt {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if ct, _ := n["class_type"].(string); ct == classType {
			return id, n, true
		}
	}
	return "", nil, false
}

// Exists reports whether a node of the given class exists.
func Exists(prompt Prompt, classType string) bool {
	_, _, ok := FindNodeByClass(prompt, classType)
	return ok
}

// Inputs returns the inputs map of a node (creating one if absent is not done
// here to avoid surprising mutations). Returns nil if missing.
func Inputs(n map[string]interface{}) map[string]interface{} {
	in, ok := n["inputs"].(map[string]interface{})
	if !ok {
		return nil
	}
	return in
}

// SetFirstByClass sets inputs[key]=value on the first node of classType. It
// returns the node id on success. If the node or its inputs are missing it
// returns ("", false).
func SetFirstByClass(prompt Prompt, classType, key string, value interface{}) (string, bool) {
	id, n, ok := FindNodeByClass(prompt, classType)
	if !ok {
		return "", false
	}
	in := Inputs(n)
	if in == nil {
		in = map[string]interface{}{}
		n["inputs"] = in
	}
	in[key] = value
	return id, true
}

// ReplaceAllLoadImage sets inputs["image"]=imageName on every LoadImage node and
// returns the patched node ids.
func ReplaceAllLoadImage(prompt Prompt, imageName string) []string {
	var updated []string
	for id, raw := range prompt {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if ct, _ := n["class_type"].(string); ct != "LoadImage" {
			continue
		}
		in := Inputs(n)
		if in == nil {
			in = map[string]interface{}{}
			n["inputs"] = in
		}
		in["image"] = imageName
		updated = append(updated, id)
	}
	return updated
}

// ApplyPositivePrompt patches the positive prompt text. It mirrors cc.py exactly:
// iterate CLIPTextEncode and TextEncodeQwenImageEditPlus nodes, skip those whose
// title contains "negative", and override the "text"/"prompt" input when the
// title contains "positive" or the input already holds non-empty text. Returns
// change strings ("prompt -> node N") and an error if no node was patched.
func ApplyPositivePrompt(prompt Prompt, text string) ([]string, error) {
	changes := []string{}
	updated := 0
	for id, raw := range prompt {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		ct, _ := n["class_type"].(string)
		if ct != "CLIPTextEncode" && ct != "TextEncodeQwenImageEditPlus" {
			continue
		}
		title := ""
		if meta, ok := n["_meta"].(map[string]interface{}); ok {
			if t, ok := meta["title"].(string); ok {
				title = strings.ToLower(t)
			}
		}
		in := Inputs(n)
		if in == nil {
			continue
		}
		if ct == "CLIPTextEncode" {
			txt, _ := in["text"].(string)
			if _, has := in["text"]; !has {
				continue
			}
			if strings.Contains(title, "negative") {
				continue
			}
			if strings.Contains(title, "positive") || strings.TrimSpace(txt) != "" {
				in["text"] = text
				updated++
				changes = append(changes, fmt.Sprintf("prompt -> node %s", id))
			}
			continue
		}
		// TextEncodeQwenImageEditPlus
		p, _ := in["prompt"].(string)
		if _, has := in["prompt"]; !has {
			continue
		}
		if strings.Contains(title, "negative") {
			continue
		}
		if strings.Contains(title, "positive") || strings.TrimSpace(p) != "" {
			in["prompt"] = text
			updated++
			changes = append(changes, fmt.Sprintf("prompt -> node %s", id))
		}
	}
	if updated == 0 {
		return nil, fmt.Errorf("Could not find a positive prompt encoding node to override prompt text.")
	}
	return changes, nil
}

// --- Pre-defined override rules (match cc.py exactly) ---

// PositivePrompt applies the positive-prompt override.
func PositivePrompt(prompt Prompt, text string) ([]string, error) {
	return ApplyPositivePrompt(prompt, text)
}

// MeshSeed -> Trellis2MeshWithVoxelAdvancedGenerator.seed
func MeshSeed(prompt Prompt, v int) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "Trellis2MeshWithVoxelAdvancedGenerator", "seed", v)
	if !ok {
		return nil, fmt.Errorf("Could not find Trellis2MeshWithVoxelAdvancedGenerator for mesh_seed override.")
	}
	return []string{fmt.Sprintf("mesh_seed=%d -> node %s", v, id)}, nil
}

// TargetFaceNum -> Trellis2SimplifyMesh.target_face_num
func TargetFaceNum(prompt Prompt, v int) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "Trellis2SimplifyMesh", "target_face_num", v)
	if !ok {
		return nil, fmt.Errorf("Could not find Trellis2SimplifyMesh for target_face_num override.")
	}
	return []string{fmt.Sprintf("target_face_num=%d -> node %s", v, id)}, nil
}

// FilenamePrefix -> Trellis2ExportMesh.filename_prefix
func FilenamePrefix(prompt Prompt, v string) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "Trellis2ExportMesh", "filename_prefix", v)
	if !ok {
		return nil, fmt.Errorf("Could not find Trellis2ExportMesh for filename_prefix override.")
	}
	return []string{fmt.Sprintf("filename_prefix=%s -> node %s", v, id)}, nil
}

// TextureSeed -> Trellis2MeshTexturing.seed
func TextureSeed(prompt Prompt, v int) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "Trellis2MeshTexturing", "seed", v)
	if !ok {
		return nil, fmt.Errorf("Could not find Trellis2MeshTexturing for texture_seed override.")
	}
	return []string{fmt.Sprintf("texture_seed=%d -> node %s", v, id)}, nil
}

// Seed -> KSampler.seed
func Seed(prompt Prompt, v int) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "KSampler", "seed", v)
	if !ok {
		return nil, fmt.Errorf("Could not find KSampler for seed override.")
	}
	return []string{fmt.Sprintf("seed=%d -> node %s", v, id)}, nil
}

// SaveImagePrefix -> SaveImage.filename_prefix
func SaveImagePrefix(prompt Prompt, v string) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "SaveImage", "filename_prefix", v)
	if !ok {
		return nil, fmt.Errorf("Could not find SaveImage for filename_prefix override.")
	}
	return []string{fmt.Sprintf("filename_prefix=%s -> node %s", v, id)}, nil
}

// ImagePrefixStage1 -> SaveImage.filename_prefix (text-to-glb stage 1). The
// change string uses "image_filename_prefix" to mirror cc.py.
func ImagePrefixStage1(prompt Prompt, v string) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "SaveImage", "filename_prefix", v)
	if !ok {
		return nil, fmt.Errorf("Could not find SaveImage for image_filename_prefix override.")
	}
	return []string{fmt.Sprintf("image_filename_prefix=%s -> node %s", v, id)}, nil
}

// Mesh -> Hy3DUploadMesh.mesh
func Mesh(prompt Prompt, v string) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "Hy3DUploadMesh", "mesh", v)
	if !ok {
		return nil, fmt.Errorf("Could not find Hy3DUploadMesh for mesh override.")
	}
	return []string{fmt.Sprintf("mesh=%s -> node %s", v, id)}, nil
}

// FBXName -> MIAAutoRig.fbx_name
func FBXName(prompt Prompt, v string) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "MIAAutoRig", "fbx_name", v)
	if !ok {
		return nil, fmt.Errorf("Could not find MIAAutoRig for fbx_name override.")
	}
	return []string{fmt.Sprintf("fbx_name=%s -> node %s", v, id)}, nil
}

// NoFingers -> MIAAutoRig.no_fingers
func NoFingers(prompt Prompt, v bool) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "MIAAutoRig", "no_fingers", v)
	if !ok {
		return nil, fmt.Errorf("Could not find MIAAutoRig for no_fingers override.")
	}
	return []string{fmt.Sprintf("no_fingers=%s -> node %s", formatVal(v), id)}, nil
}

// UseNormal -> MIAAutoRig.use_normal
func UseNormal(prompt Prompt, v bool) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "MIAAutoRig", "use_normal", v)
	if !ok {
		return nil, fmt.Errorf("Could not find MIAAutoRig for use_normal override.")
	}
	return []string{fmt.Sprintf("use_normal=%s -> node %s", formatVal(v), id)}, nil
}

// ResetToRest -> MIAAutoRig.reset_to_rest
func ResetToRest(prompt Prompt, v bool) ([]string, error) {
	id, ok := SetFirstByClass(prompt, "MIAAutoRig", "reset_to_rest", v)
	if !ok {
		return nil, fmt.Errorf("Could not find MIAAutoRig for reset_to_rest override.")
	}
	return []string{fmt.Sprintf("reset_to_rest=%s -> node %s", formatVal(v), id)}, nil
}

// ImageRef records a LoadImage patch as a change string:
// "image={ref} -> nodes {a, b}".
func ImageRef(ref string, nodeIDs []string) string {
	return fmt.Sprintf("image=%s -> nodes %s", ref, strings.Join(nodeIDs, ", "))
}

// MeshUpload records a mesh upload as a change string:
// "mesh_upload={path} -> {ref}".
func MeshUpload(path, ref string) string {
	return fmt.Sprintf("mesh_upload=%s -> %s", path, ref)
}

// Apply is the canonical cc.py _apply_overrides: it applies the shared override
// set (positive_prompt, mesh_seed, target_face_num, filename_prefix,
// texture_seed) and returns the aggregated change strings. positive_prompt of ""
// is treated as "no override" to keep call sites simple; pass "" only when no
// prompt override is intended.
//
// Note: cc.py distinguishes "no prompt override" (None) from empty-string
// prompts. Since an empty prompt is meaningless here, "" means "no override".
func Apply(prompt Prompt, positivePrompt string, meshSeed, targetFaceNum, textureSeed int, hasMeshSeed, hasTargetFaceNum, hasTextureSeed bool, filenamePrefix string) ([]string, error) {
	changes := []string{}
	if positivePrompt != "" {
		c, err := ApplyPositivePrompt(prompt, positivePrompt)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c...)
	}
	if hasMeshSeed {
		c, err := MeshSeed(prompt, meshSeed)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c...)
	}
	if hasTargetFaceNum {
		c, err := TargetFaceNum(prompt, targetFaceNum)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c...)
	}
	if filenamePrefix != "" {
		c, err := FilenamePrefix(prompt, filenamePrefix)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c...)
	}
	if hasTextureSeed {
		c, err := TextureSeed(prompt, textureSeed)
		if err != nil {
			return nil, err
		}
		changes = append(changes, c...)
	}
	return changes, nil
}
