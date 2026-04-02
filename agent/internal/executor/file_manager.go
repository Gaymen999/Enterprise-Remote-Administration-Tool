package executor

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxFileSize = 100 * 1024 * 1024
	chunkSize          = 64 * 1024
)

type FileManager struct {
	allowedDirs []string
	maxFileSize int64
}

type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
	IsDir   bool   `json:"is_dir"`
	IsLink  bool   `json:"is_link"`
}

type FileOperationResult struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	RequestID string      `json:"request_id"`
}

func NewFileManager() *FileManager {
	envDirs := os.Getenv("ALLOWED_DIRS")
	allowedDirs := parseAllowedDirs(envDirs)
	maxFileSize := parseMaxFileSize()

	return &FileManager{
		allowedDirs: allowedDirs,
		maxFileSize: maxFileSize,
	}
}

func parseAllowedDirs(env string) []string {
	if env == "" {
		if isWindows() {
			return []string{"C:\\", "D:\\"}
		}
		return []string{"/", "/home", "/tmp", "/var", "/etc"}
	}

	dirs := strings.Split(env, ",")
	result := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d != "" {
			result = append(result, d)
		}
	}
	return result
}

func parseMaxFileSize() int64 {
	env := os.Getenv("MAX_FILE_SIZE")
	if env == "" {
		return defaultMaxFileSize
	}
	var size int64
	fmt.Sscanf(env, "%d", &size)
	if size <= 0 {
		return defaultMaxFileSize
	}
	return size
}

func (fm *FileManager) HandleFileOperation(payload map[string]interface{}) *FileOperationResult {
	operation, _ := payload["operation"].(string)
	path, _ := payload["path"].(string)
	requestID, _ := payload["request_id"].(string)

	if operation == "" || path == "" {
		return &FileOperationResult{
			Success:   false,
			Error:     "operation and path are required",
			RequestID: requestID,
		}
	}

	if !fm.isPathAllowed(path) {
		return &FileOperationResult{
			Success:   false,
			Error:     "access denied: path outside allowed directories",
			RequestID: requestID,
		}
	}

	switch operation {
	case "list":
		return fm.listDirectory(path, requestID)
	case "stat":
		return fm.statFile(path, requestID)
	case "download":
		return fm.downloadFile(path, requestID)
	case "upload":
		content, _ := payload["content"].(string)
		return fm.uploadFile(path, content, requestID)
	case "delete":
		return fm.deleteFile(path, requestID)
	case "mkdir":
		return fm.createDirectory(path, requestID)
	default:
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("unknown operation: %s", operation),
			RequestID: requestID,
		}
	}
}

func (fm *FileManager) isPathAllowed(path string) bool {
	cleanPath := filepath.Clean(path)

	if cleanPath == "" || cleanPath == "." {
		return false
	}

	if strings.Contains(cleanPath, "..") {
		return false
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	if isWindows() {
		absPath = strings.ReplaceAll(absPath, "/", "\\")
		absPath = strings.ToLower(absPath)
	}

	for _, allowed := range fm.allowedDirs {
		allowedClean := filepath.Clean(allowed)
		allowedAbs, err := filepath.Abs(allowedClean)
		if err != nil {
			continue
		}
		if isWindows() {
			allowedAbs = strings.ReplaceAll(allowedAbs, "/", "\\")
			allowedAbs = strings.ToLower(allowedAbs)
		}

		if strings.HasPrefix(absPath, allowedAbs) {
			return true
		}
	}

	return false
}

func (fm *FileManager) listDirectory(path string, requestID string) *FileOperationResult {
	entries, err := os.ReadDir(path)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to read directory: %v", err),
			RequestID: requestID,
		}
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileInfo := FileInfo{
			Name:    entry.Name(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:   entry.IsDir(),
			IsLink:  entry.Type()&os.ModeSymlink != 0,
		}
		files = append(files, fileInfo)
	}

	return &FileOperationResult{
		Success:   true,
		Data:      files,
		RequestID: requestID,
	}
}

func (fm *FileManager) statFile(path string, requestID string) *FileOperationResult {
	info, err := os.Stat(path)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to stat file: %v", err),
			RequestID: requestID,
		}
	}

	fileInfo := FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		IsDir:   info.IsDir(),
		IsLink:  info.Mode()&os.ModeSymlink != 0,
	}

	return &FileOperationResult{
		Success:   true,
		Data:      fileInfo,
		RequestID: requestID,
	}
}

func (fm *FileManager) downloadFile(path string, requestID string) *FileOperationResult {
	info, err := os.Stat(path)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to stat file: %v", err),
			RequestID: requestID,
		}
	}

	if info.IsDir() {
		return &FileOperationResult{
			Success:   false,
			Error:     "cannot download directory",
			RequestID: requestID,
		}
	}

	if info.Size() > fm.maxFileSize {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("file too large: %d bytes (max: %d)", info.Size(), fm.maxFileSize),
			RequestID: requestID,
		}
	}

	data, err := fm.readFileBuffered(path)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to read file: %v", err),
			RequestID: requestID,
		}
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	return &FileOperationResult{
		Success: true,
		Data: map[string]interface{}{
			"content":       encoded,
			"size":          info.Size(),
			"name":          filepath.Base(path),
			"original_path": path,
		},
		RequestID: requestID,
	}
}

func (fm *FileManager) uploadFile(path string, contentB64 string, requestID string) *FileOperationResult {
	if !fm.isPathAllowed(path) {
		return &FileOperationResult{
			Success:   false,
			Error:     "access denied: path outside allowed directories",
			RequestID: requestID,
		}
	}

	data, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to decode content: %v", err),
			RequestID: requestID,
		}
	}

	if int64(len(data)) > fm.maxFileSize {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("content too large: %d bytes (max: %d)", len(data), fm.maxFileSize),
			RequestID: requestID,
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create directory: %v", err),
			RequestID: requestID,
		}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to write file: %v", err),
			RequestID: requestID,
		}
	}

	return &FileOperationResult{
		Success:   true,
		Data:      map[string]interface{}{"path": path, "size": len(data)},
		RequestID: requestID,
	}
}

func (fm *FileManager) deleteFile(path string, requestID string) *FileOperationResult {
	if !fm.isPathAllowed(path) {
		return &FileOperationResult{
			Success:   false,
			Error:     "access denied: cannot delete outside allowed directories",
			RequestID: requestID,
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("file not found: %v", err),
			RequestID: requestID,
		}
	}

	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return &FileOperationResult{
				Success:   false,
				Error:     fmt.Sprintf("failed to delete directory: %v", err),
				RequestID: requestID,
			}
		}
	} else {
		if err := os.Remove(path); err != nil {
			return &FileOperationResult{
				Success:   false,
				Error:     fmt.Sprintf("failed to delete file: %v", err),
				RequestID: requestID,
			}
		}
	}

	return &FileOperationResult{
		Success:   true,
		Data:      map[string]interface{}{"deleted": path},
		RequestID: requestID,
	}
}

func (fm *FileManager) createDirectory(path string, requestID string) *FileOperationResult {
	if !fm.isPathAllowed(path) {
		return &FileOperationResult{
			Success:   false,
			Error:     "access denied: cannot create directory outside allowed directories",
			RequestID: requestID,
		}
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return &FileOperationResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create directory: %v", err),
			RequestID: requestID,
		}
	}

	return &FileOperationResult{
		Success:   true,
		Data:      map[string]interface{}{"created": path},
		RequestID: requestID,
	}
}

func (fm *FileManager) GetAllowedDirectories() []string {
	return fm.allowedDirs
}

func ReadFileChunk(path string, offset int64, size int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	buffer := make([]byte, size)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return buffer[:n], nil
}

func (fm *FileManager) readFileBuffered(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := make([]byte, 0, 4096)
	buf := make([]byte, chunkSize)

	for {
		n, err := file.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
			if len(data) > int(fm.maxFileSize) {
				return nil, fmt.Errorf("file too large during read")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}
