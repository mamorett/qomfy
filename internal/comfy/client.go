// Package comfy is a thin HTTP/WebSocket client for a ComfyUI server. It mirrors
// the Python client in cc.py: /prompt, /history, /queue, /system_stats,
// /upload/image, /view, and a WebSocket progress stream.
package comfy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Client wraps a ComfyUI base URL and an HTTP client.
type Client struct {
	Base      string
	HTTP      *http.Client
	UserAgent string
}

// NewClient builds a Client from a base URL (trailing slash trimmed) and an
// overall HTTP timeout.
func NewClient(base string, timeout time.Duration) *Client {
	base = strings.TrimRight(base, "/")
	if base == "" {
		base = "http://localhost:8188"
	}
	return &Client{
		Base:      base,
		HTTP:      &http.Client{Timeout: timeout},
		UserAgent: "qomfy/1.0",
	}
}

func (c *Client) getJSON(u string) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

func (c *Client) doJSON(req *http.Request) (map[string]interface{}, error) {
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ComfyUI request to %s failed: %s: %s", req.URL.Path, resp.Status, strings.TrimSpace(string(body)))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("invalid JSON from %s: %w", req.URL.Path, err)
	}
	return out, nil
}

// Health returns the /system_stats payload.
func (c *Client) Health() (map[string]interface{}, error) {
	return c.getJSON(c.Base + "/system_stats")
}

// SubmitPrompt POSTs the prompt payload and returns the parsed response (which
// must contain "prompt_id").
func (c *Client) SubmitPrompt(prompt map[string]interface{}, clientID string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"prompt":    prompt,
		"client_id": clientID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.Base+"/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ComfyUI /prompt failed: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("invalid JSON from /prompt: %w", err)
	}
	return out, nil
}

// Queue returns the /queue state.
func (c *Client) Queue() (map[string]interface{}, error) {
	return c.getJSON(c.Base + "/queue")
}

// History returns the raw /history/{id} payload.
func (c *Client) History(promptID string) (map[string]interface{}, error) {
	u := c.Base + "/history/" + url.PathEscape(promptID)
	return c.getJSON(u)
}

// HistoryItem normalizes the /history response into the item for promptID:
// either history[promptID] or the object itself if it has "outputs".
func (c *Client) HistoryItem(promptID string) (map[string]interface{}, error) {
	raw, err := c.History(promptID)
	if err != nil {
		return nil, err
	}
	if m, ok := raw[promptID].(map[string]interface{}); ok {
		return m, nil
	}
	if _, ok := raw["outputs"]; ok {
		return raw, nil
	}
	return nil, nil
}

// UploadImage uploads a local file to ComfyUI's input directory and returns a
// reference string (optionally "{subfolder}/{name}"). It mirrors cc.py's
// _upload_input_image/_upload_input_asset.
func (c *Client) UploadImage(filePath string, overwrite bool) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("file not found: %s", filePath)
	}
	defer f.Close()

	contentType := "application/octet-stream"
	if ct := detectContentType(filePath); ct != "" {
		contentType = ct
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename=%q`, filepath.Base(filePath)))
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	_ = mw.WriteField("overwrite", fmt.Sprintf("%v", overwrite))
	_ = mw.WriteField("type", "input")
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.Base+"/upload/image", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ComfyUI upload failed: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("invalid upload response: %w", err)
	}
	name, _ := payload["name"].(string)
	if name == "" {
		return "", fmt.Errorf("Unexpected file upload response: %s", string(mustJSON(payload)))
	}
	if sub, _ := payload["subfolder"].(string); sub != "" {
		return sub + "/" + name, nil
	}
	return name, nil
}

func detectContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".glb":
		return "model/gltf-binary"
	case ".gltf":
		return "model/gltf+json"
	case ".fbx":
		return "application/octet-stream"
	default:
		return ""
	}
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// DownloadRef writes the bytes referenced by ref into dest. If ref is a full
// http(s) URL it is fetched directly; otherwise the ComfyUI /view endpoint is
// used (type=output, optional subfolder).
func (c *Client) DownloadRef(ref string, dest string) error {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequest(http.MethodGet, ref, nil)
		if err != nil {
			return err
		}
		return c.downloadTo(req, dest)
	}
	refPath := path.Clean(ref)
	params := url.Values{}
	params.Set("filename", path.Base(refPath))
	params.Set("type", "output")
	if parent := path.Dir(refPath); parent != "." && parent != "/" && parent != "" {
		params.Set("subfolder", parent)
	}
	u := c.Base + "/view?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.downloadTo(req, dest)
}

func (c *Client) downloadTo(req *http.Request, dest string) error {
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
