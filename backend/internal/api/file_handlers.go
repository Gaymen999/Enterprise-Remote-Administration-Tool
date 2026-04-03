package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/enterprise-rat/backend/internal/ws"
	"github.com/go-chi/chi/v5"
)

var (
	sandboxBaseDir = getSandboxBaseDir()
	maxUploadSize  = 10 * 1024 * 1024
)

func getSandboxBaseDir() string {
	if baseDir := os.Getenv("SANDBOX_BASE_DIR"); baseDir != "" {
		return baseDir
	}
	if runtimeGOOS() == "windows" {
		return "C:\\var\\lib\\enterprise-rat\\sandbox"
	}
	return "/var/lib/enterprise-rat/sandbox"
}

func validateSandboxDir() error {
	info, err := os.Stat(sandboxBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("sandbox directory does not exist: %s", sandboxBaseDir)
		}
		return fmt.Errorf("cannot access sandbox directory: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sandbox path is not a directory: %s", sandboxBaseDir)
	}
	return nil
}

func secureResolvePath(userInput string) (string, error) {
	userInput = strings.TrimSpace(userInput)
	userInput = strings.ReplaceAll(userInput, "\x00", "")

	cleanPath := filepath.Clean(userInput)

	if cleanPath == "." || cleanPath == "" {
		return "", &PathTraversalError{Message: "invalid path: empty or current directory"}
	}

	absBaseDir, err := filepath.Abs(sandboxBaseDir)
	if err != nil {
		return "", &PathTraversalError{Message: "failed to resolve base directory"}
	}

	joinedPath := filepath.Join(absBaseDir, cleanPath)
	cleanedFullPath := filepath.Clean(joinedPath)

	if runtimeGOOS() == "windows" {
		absBaseDir = strings.ToLower(strings.ReplaceAll(absBaseDir, "/", "\\"))
		cleanedFullPath = strings.ToLower(strings.ReplaceAll(cleanedFullPath, "/", "\\"))
	}

	if !strings.HasPrefix(cleanedFullPath, absBaseDir) {
		return "", &PathTraversalError{Message: "access denied: path outside sandbox directory"}
	}

	// Return path relative to sandboxBaseDir so the agent can resolve it against its own sandbox
	relPath, err := filepath.Rel(absBaseDir, cleanedFullPath)
	if err != nil {
		return "", &PathTraversalError{Message: "failed to make path relative"}
	}

	return relPath, nil
}

type PathTraversalError struct {
	Message string
}

func (e *PathTraversalError) Error() string {
	return e.Message
}

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

		sanitizedPath, err := secureResolvePath(req.Path)
		if err != nil {
			http.Error(w, `{"error": "path validation failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

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

		sanitizedPath, err := secureResolvePath(filePath)
		if err != nil {
			http.Error(w, `{"error": "path validation failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

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

		sanitizedPath, err := secureResolvePath(path)
		if err != nil {
			http.Error(w, `{"error": "path validation failed: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		if r.ContentLength > int64(maxUploadSize) {
			http.Error(w, `{"error": "file too large (max 10MB)"}`, http.StatusBadRequest)
			return
		}

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
	return base64.StdEncoding.EncodeToString(data)
}

func runtimeGOOS() string {
	return runtime.GOOS
}

func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
			b[i] = letters[num.Int64()]
		}
		return string(b)
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}
