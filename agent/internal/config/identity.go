package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
)

type Identity struct {
	AgentID   string `json:"agent_id"`
	Hostname  string `json:"hostname"`
	OSFamily  string `json:"os_family"`
	OSVersion string `json:"os_version"`
}

func LoadOrCreateIdentity() (*Identity, error) {
	idFile := ".agent_id"

	data, err := os.ReadFile(idFile)
	if err == nil && len(data) > 0 {
		var identity Identity
		if err := json.Unmarshal(data, &identity); err == nil {
			return &identity, nil
		}
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	identity := &Identity{
		AgentID:   generateUUID(),
		Hostname:  hostname,
		OSFamily:  runtime.GOOS,
		OSVersion: getOSVersion(),
	}

	if data, err := json.Marshal(identity); err == nil {
		os.WriteFile(idFile, data, 0600)
	}

	return identity, nil
}

func generateUUID() string {
	out, err := exec.Command("powershell", "-Command", "[guid]::NewGuid().ToString()").Output()
	if err == nil && len(out) > 0 {
		return string(out[:len(out)-2])
	}

	out, err = exec.Command("uuidgen").Output()
	if err == nil && len(out) > 0 {
		return string(out[:len(out)-1])
	}

	return "fallback-" + randomString(16)
}

func getOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		out, _ := exec.Command("powershell", "-Command", "(Get-WmiObject Win32_OperatingSystem).Version").Output()
		if len(out) > 0 {
			return string(out[:len(out)-2])
		}
	case "linux":
		out, _ := exec.Command("cat", "/etc/os-release").Output()
		if len(out) > 0 {
			return string(out)
		}
	case "darwin":
		out, _ := exec.Command("sw_vers", "-productVersion").Output()
		if len(out) > 0 {
			return string(out[:len(out)-1])
		}
	}
	return "unknown"
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
