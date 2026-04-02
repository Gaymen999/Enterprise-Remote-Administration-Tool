package models

import (
	"time"
)

type Agent struct {
	ID           string     `json:"id"`
	Hostname     string     `json:"hostname"`
	IPAddress    string     `json:"ip_address"`
	OSFamily     string     `json:"os_family"`
	OSVersion    string     `json:"os_version"`
	AgentVersion string     `json:"agent_version"`
	LastSeen     *time.Time `json:"last_seen"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Command struct {
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	UserID         string    `json:"user_id"`
	CommandPayload string    `json:"command_payload"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CommandResult struct {
	ID          string    `json:"id"`
	CommandID   string    `json:"command_id"`
	StdOut      string    `json:"stdout"`
	StdErr      string    `json:"stderr"`
	ExitCode    int       `json:"exit_code"`
	CompletedAt time.Time `json:"completed_at"`
}

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}
