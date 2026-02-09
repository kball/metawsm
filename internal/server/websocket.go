package server

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func (r *Runtime) handleForumStream(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	ticket := strings.TrimSpace(req.URL.Query().Get("ticket"))
	cursor, err := parseInt64Query(req.URL.Query().Get("cursor"), 0)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	limit, err := parseIntQuery(req.URL.Query().Get("limit"), 100)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	pollMS, err := parseIntQuery(req.URL.Query().Get("poll_ms"), 1000)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_poll_ms", err.Error())
		return
	}
	if pollMS <= 0 {
		pollMS = 1000
	}

	conn, err := upgradeWebSocket(w, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "websocket_upgrade_failed", err.Error())
		return
	}
	defer conn.Close()

	if err := r.streamForumEvents(conn, ticket, cursor, limit, time.Duration(pollMS)*time.Millisecond); err != nil {
		_ = writeWebSocketJSON(conn, map[string]any{
			"type":    "error",
			"message": err.Error(),
		})
	}
}

func (r *Runtime) streamForumEvents(conn net.Conn, ticket string, cursor int64, limit int, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	nextCursor := cursor
	for {
		events, err := r.service.ForumWatchEvents(ticket, nextCursor, limit)
		if err != nil {
			return err
		}
		if len(events) > 0 {
			nextCursor = events[len(events)-1].Sequence
		}
		frame := map[string]any{
			"type":        "forum.events",
			"events":      events,
			"next_cursor": nextCursor,
			"sent_at":     time.Now().UTC().Format(time.RFC3339Nano),
		}
		if len(events) == 0 {
			frame["type"] = "heartbeat"
		}
		if err := writeWebSocketJSON(conn, frame); err != nil {
			return err
		}
		if len(events) > 0 {
			continue
		}
		<-ticker.C
	}
}

func upgradeWebSocket(w http.ResponseWriter, req *http.Request) (net.Conn, error) {
	if !headerContainsToken(req.Header.Get("Connection"), "upgrade") {
		return nil, fmt.Errorf("connection header must include Upgrade")
	}
	if !strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket") {
		return nil, fmt.Errorf("upgrade header must be websocket")
	}
	if strings.TrimSpace(req.Header.Get("Sec-WebSocket-Version")) != "13" {
		return nil, fmt.Errorf("sec-websocket-version must be 13")
	}
	websocketKey := strings.TrimSpace(req.Header.Get("Sec-WebSocket-Key"))
	if websocketKey == "" {
		return nil, fmt.Errorf("sec-websocket-key is required")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("response writer does not support hijacking")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	accept := websocketAcceptKey(websocketKey)
	response := strings.Builder{}
	response.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	response.WriteString("Upgrade: websocket\r\n")
	response.WriteString("Connection: Upgrade\r\n")
	response.WriteString("Sec-WebSocket-Accept: ")
	response.WriteString(accept)
	response.WriteString("\r\n")
	response.WriteString("\r\n")
	if _, err := rw.WriteString(response.String()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func websocketAcceptKey(key string) string {
	hash := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func writeWebSocketJSON(conn net.Conn, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeWebSocketFrame(conn, 0x1, body)
}

func writeWebSocketFrame(conn net.Conn, opcode byte, payload []byte) error {
	header := make([]byte, 0, 10)
	header = append(header, 0x80|opcode)
	size := len(payload)
	switch {
	case size <= 125:
		header = append(header, byte(size))
	case size <= 65535:
		header = append(header, 126)
		header = append(header, byte(size>>8), byte(size))
	default:
		header = append(header, 127)
		extended := make([]byte, 8)
		binary.BigEndian.PutUint64(extended, uint64(size))
		header = append(header, extended...)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if _, err := conn.Write(append(header, payload...)); err != nil {
		return err
	}
	return nil
}

func headerContainsToken(header string, token string) bool {
	parts := strings.Split(header, ",")
	for _, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), strings.TrimSpace(token)) {
			return true
		}
	}
	return false
}

func parsePositiveInt(raw string, fallback int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
