package comfy

import (
	"net"
	"os"

	"github.com/gorilla/websocket"
)

func osMkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// wsDial connects to a ComfyUI WebSocket URL.
func wsDial(urlStr string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(urlStr, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(net.Error); ok {
		return ne.Timeout()
	}
	return false
}
