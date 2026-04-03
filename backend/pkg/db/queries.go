package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/enterprise-rat/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")
var ErrCommandNotFound = errors.New("command not found")

func GetAgents(ctx context.Context, pool *pgxpool.Pool) ([]models.Agent, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, hostname, 
			COALESCE(ip_address::text, '') as ip_address,
			COALESCE(os_family, '') as os_family,
			COALESCE(os_version, '') as os_version,
			COALESCE(agent_version, '') as agent_version,
			last_seen, status, created_at, updated_at
		FROM agents 
		ORDER BY hostname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var agent models.Agent
		err := rows.Scan(
			&agent.ID, &agent.Hostname, &agent.IPAddress,
			&agent.OSFamily, &agent.OSVersion, &agent.AgentVersion,
			&agent.LastSeen, &agent.Status, &agent.CreatedAt, &agent.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}

	if agents == nil {
		agents = []models.Agent{}
	}

	return agents, nil
}

func GetUserByUsername(ctx context.Context, pool *pgxpool.Pool, username string) (string, string, error) {
	var userID, passwordHash string
	query := `SELECT id, password_hash FROM users WHERE username = $1 AND is_active = true`
	err := pool.QueryRow(ctx, query, username).Scan(&userID, &passwordHash)
	if err != nil {
		return "", "", ErrUserNotFound
	}
	return userID, passwordHash, nil
}

func GetUserByUsernameAndRole(ctx context.Context, pool *pgxpool.Pool, username string) (string, string, string, error) {
	var userID, passwordHash, role string
	query := `SELECT id, password_hash, role FROM users WHERE username = $1 AND is_active = true`
	err := pool.QueryRow(ctx, query, username).Scan(&userID, &passwordHash, &role)
	if err != nil {
		return "", "", "", ErrUserNotFound
	}
	return userID, passwordHash, role, nil
}

func SaveCommandResult(ctx context.Context, pool *pgxpool.Pool, commandID, stdout, stderr string, exitCode int) error {
	query := `
		INSERT INTO command_results (id, command_id, stdout, stderr, exit_code, completed_at)
		VALUES (gen_random_uuid()::uuid, $1, $2, $3, $4, NOW())
		ON CONFLICT (command_id) DO UPDATE SET
			stdout = EXCLUDED.stdout,
			stderr = EXCLUDED.stderr,
			exit_code = EXCLUDED.exit_code,
			completed_at = EXCLUDED.completed_at
	`
	_, err := pool.Exec(ctx, query, commandID, stdout, stderr, exitCode)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `UPDATE commands SET status = 'completed', updated_at = NOW() WHERE id = $1`, commandID)
	return err
}

func CreateCommand(ctx context.Context, pool *pgxpool.Pool, agentID, userID, executable string, args []string, timeout int) (string, error) {
	commandPayload := map[string]interface{}{
		"executable": sanitizeCommandPayload(executable),
		"args":       sanitizeArgs(args),
	}

	payloadJSON, err := json.Marshal(commandPayload)
	if err != nil {
		return "", err
	}

	var commandID string
	err = pool.QueryRow(ctx, `
		INSERT INTO commands (id, agent_id, user_id, command_payload, status, created_at, updated_at)
		VALUES (gen_random_uuid()::uuid, $1, $2, $3, 'pending', NOW(), NOW())
		RETURNING id
	`, agentID, userID, string(payloadJSON)).Scan(&commandID)

	if err != nil {
		return "", err
	}
	return commandID, nil
}

func sanitizeCommandPayload(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.ReplaceAll(cmd, ";", "")
	cmd = strings.ReplaceAll(cmd, "&", "")
	cmd = strings.ReplaceAll(cmd, "|", "")
	cmd = strings.ReplaceAll(cmd, "`", "")
	cmd = strings.ReplaceAll(cmd, "$", "")
	cmd = strings.ReplaceAll(cmd, "(", "")
	cmd = strings.ReplaceAll(cmd, ")", "")
	return cmd
}

func sanitizeArgs(args []string) []string {
	sanitized := make([]string, 0, len(args))
	for _, arg := range args {
		sanitized = append(sanitized, sanitizeCommandPayload(arg))
	}
	return sanitized
}

func GetCommandByID(ctx context.Context, pool *pgxpool.Pool, commandID string) (*models.Command, error) {
	var command models.Command
	var payloadJSON []byte
	query := `SELECT id, agent_id, user_id, command_payload, status, created_at, updated_at FROM commands WHERE id = $1`
	err := pool.QueryRow(ctx, query, commandID).Scan(
		&command.ID, &command.AgentID, &command.UserID, &payloadJSON,
		&command.Status, &command.CreatedAt, &command.UpdatedAt,
	)
	if err != nil {
		return nil, ErrCommandNotFound
	}
	command.CommandPayload = string(payloadJSON)
	return &command, nil
}

type CommandResult struct {
	CommandID string
	Stdout    string
	Stderr    string
	ExitCode  int
	Completed *string
}

func GetCommandResult(ctx context.Context, pool *pgxpool.Pool, commandID string) (*CommandResult, error) {
	var result CommandResult
	query := `SELECT command_id, stdout, stderr, exit_code, completed_at FROM command_results WHERE command_id = $1`
	err := pool.QueryRow(ctx, query, commandID).Scan(
		&result.CommandID, &result.Stdout, &result.Stderr, &result.ExitCode, &result.Completed,
	)
	if err != nil {
		return nil, ErrCommandNotFound
	}
	return &result, nil
}
