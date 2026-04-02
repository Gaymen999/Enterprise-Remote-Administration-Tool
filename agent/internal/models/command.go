package models

type CommandRequest struct {
	CommandID       string   `json:"command_id"`
	Executable     string   `json:"executable"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type CommandResponse struct {
	CommandID string `json:"command_id"`
	StdOut    string `json:"stdout"`
	StdErr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	ErrorMsg  string `json:"error_msg,omitempty"`
}
