package comfy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// GLBExtensions matches cc.py's GLB_EXTENSIONS.
var GLBExtensions = map[string]bool{".glb": true}

// ImageExtensions matches cc.py's IMAGE_EXTENSIONS.
var ImageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

func hasExt(name string, exts map[string]bool) bool {
	return exts[strings.ToLower(path.Ext(name))]
}

// ExtractFileRefs walks a history item's outputs collecting refs whose filename
// matches one of exts. A ref is either a bare string matching an extension, or a
// dict with a "filename" key matching an extension (prefixed with its subfolder).
// This mirrors cc.py's _extract_file_refs verbatim.
func ExtractFileRefs(historyItem map[string]interface{}, exts map[string]bool) []string {
	var refs []string
	var collect func(v interface{})
	collect = func(v interface{}) {
		switch val := v.(type) {
		case string:
			if hasExt(val, exts) {
				refs = append(refs, val)
			}
		case map[string]interface{}:
			if fn, ok := val["filename"].(string); ok && hasExt(fn, exts) {
				if sf, _ := val["subfolder"].(string); sf != "" {
					refs = append(refs, sf+"/"+fn)
				} else {
					refs = append(refs, fn)
				}
				return
			}
			for _, nested := range val {
				collect(nested)
			}
		case []interface{}:
			for _, item := range val {
				collect(item)
			}
		}
	}
	outputs, _ := historyItem["outputs"].(map[string]interface{})
	if outputs == nil {
		return refs
	}
	for _, nodeData := range outputs {
		collect(nodeData)
	}
	return refs
}

// WaitForCompletion polls /queue and /history/{id} until the prompt appears in
// history or the timeout elapses. When clientID is non-empty it concurrently
// streams progress from the WebSocket, writing formatted lines to progressFn (or
// stdout if nil). It returns the queue state and history item.
func WaitForCompletion(client *Client, promptID, clientID string, interval, timeout float64, verbose bool, progressFn func(string)) (map[string]interface{}, map[string]interface{}, error) {
	stop := make(chan struct{})
	if clientID != "" {
		go streamWSProgress(client, clientID, promptID, stop, progressFn)
	}
	defer close(stop)

	elapsed := 0.0
	for elapsed <= timeout {
		queueState, err := client.Queue()
		if err == nil {
			_ = queueState // echoed by caller
		}
		historyItem, err := client.HistoryItem(promptID)
		if err != nil {
			return nil, nil, err
		}
		if historyItem != nil {
			qs := map[string]interface{}{}
			if q, e := client.Queue(); e == nil {
				qs = q
			}
			return qs, historyItem, nil
		}
		if verbose {
			running, pending := 0, 0
			if q, err := client.Queue(); err == nil {
				if r, ok := q["queue_running"].([]interface{}); ok {
					running = len(r)
				}
				if p, ok := q["queue_pending"].([]interface{}); ok {
					pending = len(p)
				}
			}
			line := fmt.Sprintf("Waiting... running=%d pending=%d elapsed=%ds", running, pending, int(elapsed))
			if progressFn != nil {
				progressFn(line)
			} else {
				fmt.Println(line)
			}
		}
		time.Sleep(time.Duration(interval * float64(time.Second)))
		elapsed += interval
	}
	return nil, nil, fmt.Errorf("Timed out waiting for prompt_id=%s", promptID)
}

// DownloadRefs extracts refs of exts from historyItem and downloads each into
// outDir, returning the written paths.
func DownloadRefs(client *Client, promptID string, historyItem map[string]interface{}, outDir string, exts map[string]bool) ([]string, error) {
	refs := ExtractFileRefs(historyItem, exts)
	if len(refs) == 0 {
		return nil, nil
	}
	if err := ensureDir(outDir); err != nil {
		return nil, err
	}
	var downloaded []string
	for _, ref := range refs {
		defaultExt := ".bin"
		for e := range exts {
			defaultExt = e
			break
		}
		filename := path.Base(ref)
		if filename == "" || filename == "." {
			filename = promptID + defaultExt
		}
		dest := path.Join(outDir, filename)
		if err := client.DownloadRef(ref, dest); err != nil {
			return downloaded, err
		}
		downloaded = append(downloaded, dest)
	}
	return downloaded, nil
}

func ensureDir(dir string) error {
	return osMkdirAll(dir)
}

// FormatWSProgressLine mirrors cc.py's _format_ws_progress_line. It returns a
// human-readable line, or "" if the message should be skipped.
func FormatWSProgressLine(message map[string]interface{}, promptID string) string {
	msgType, _ := message["type"].(string)
	if msgType == "" {
		return ""
	}
	data, _ := message["data"].(map[string]interface{})
	if data == nil {
		data = map[string]interface{}{}
	}
	if pid, ok := data["prompt_id"].(string); ok && pid != promptID {
		return ""
	}

	switch msgType {
	case "execution_start":
		return fmt.Sprintf("WS: execution started for prompt_id=%s", promptID)
	case "executing":
		if node, ok := data["node"]; ok && node != nil {
			return fmt.Sprintf("WS: executing node %v", node)
		}
		return fmt.Sprintf("WS: execution finished for prompt_id=%s", promptID)
	case "executed":
		node, ok := data["node"]
		if !ok || node == nil {
			return ""
		}
		return fmt.Sprintf("WS: completed node %v", node)
	case "progress":
		value, vOk := data["value"].(float64)
		maxValue, mOk := data["max"].(float64)
		if vOk && mOk && int(maxValue) > 0 {
			percent := int((value / maxValue) * 100)
			return fmt.Sprintf("WS: progress %v/%v (%d%%)", int(value), int(maxValue), percent)
		}
		return ""
	case "execution_cached":
		if nodes, ok := data["nodes"].([]interface{}); ok {
			parts := make([]string, len(nodes))
			for i, n := range nodes {
				parts[i] = fmt.Sprintf("%v", n)
			}
			return fmt.Sprintf("WS: using cached outputs for nodes %s", strings.Join(parts, ", "))
		}
		return "WS: using cached outputs"
	case "execution_error":
		if em, ok := data["exception_message"].(string); ok && strings.TrimSpace(em) != "" {
			return fmt.Sprintf("WS: execution error: %s", em)
		}
		return "WS: execution error"
	case "status":
		status, ok := data["status"].(map[string]interface{})
		if !ok {
			return ""
		}
		execInfo, ok := status["exec_info"].(map[string]interface{})
		if !ok {
			return ""
		}
		if qr, ok := execInfo["queue_remaining"].(float64); ok {
			return fmt.Sprintf("WS: queue remaining=%d", int(qr))
		}
	}
	return ""
}

// streamWSProgress connects to the ComfyUI WS and emits formatted progress
// lines. It is best-effort: any failure is silently ignored, matching cc.py.
func streamWSProgress(client *Client, clientID, promptID string, stop chan struct{}, progressFn func(string)) {
	wsURL := buildWSURL(client.Base, clientID)
	conn, err := wsDial(wsURL)
	if err != nil {
		emit(progressFn, "WS: unavailable; continuing with polling.")
		return
	}
	defer conn.Close()
	emit(progressFn, fmt.Sprintf("WS: connected (%s)", wsURL))
	var lastLine string
	for {
		select {
		case <-stop:
			return
		default:
		}
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return
		}
		var message map[string]interface{}
		if err := json.Unmarshal(raw, &message); err != nil {
			continue
		}
		if line := FormatWSProgressLine(message, promptID); line != "" && line != lastLine {
			emit(progressFn, line)
			lastLine = line
		}
	}
}

func emit(fn func(string), line string) {
	if fn != nil {
		fn(line)
	} else {
		fmt.Println(line)
	}
}

func buildWSURL(base, clientID string) string {
	wsBase := base
	switch {
	case strings.HasPrefix(base, "https://"):
		wsBase = "wss://" + base[len("https://"):]
	case strings.HasPrefix(base, "http://"):
		wsBase = "ws://" + base[len("http://"):]
	}
	return wsBase + "/ws?clientId=" + url.QueryEscape(clientID)
}
