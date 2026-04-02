package db

import (
	"context"
	"errors"
	"time"

	"github.com/enterprise-rat/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAgentNotFound = errors.New("agent not found")

type AgentInfo struct {
	AgentID   string
	Hostname  string
	OSFamily  string
	OSVersion string
}

func UpsertAgent(ctx context.Context, pool *pgxpool.Pool, info AgentInfo) error {
	query := `
		INSERT INTO agents (id, hostname, os_family, os_version, status, last_seen, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'online', $5, $5, $5)
		ON CONFLICT (id) DO UPDATE SET
			status = 'online',
			last_seen = $5,
			hostname = COALESCE(EXCLUDED.hostname, agents.hostname),
			os_family = COALESCE(EXCLUDED.os_family, agents.os_family),
			os_version = COALESCE(EXCLUDED.os_version, agents.os_version),
			updated_at = $5
	`
	now := time.Now()
	_, err := pool.Exec(ctx, query, info.AgentID, info.Hostname, info.OSFamily, info.OSVersion, now)
	return err
}

func UpdateAgentStatus(ctx context.Context, pool *pgxpool.Pool, agentID, status string) error {
	query := `UPDATE agents SET status = $1, last_seen = $2, updated_at = $2 WHERE id = $3`
	now := time.Now()
	_, err := pool.Exec(ctx, query, status, now, agentID)
	return err
}

func GetAgentByID(ctx context.Context, pool *pgxpool.Pool, agentID string) (*models.Agent, error) {
	var agent models.Agent
	query := `
		SELECT id, hostname, 
			COALESCE(ip_address::text, '') as ip_address,
			COALESCE(os_family, '') as os_family,
			COALESCE(os_version, '') as os_version,
			COALESCE(agent_version, '') as agent_version,
			last_seen, status, created_at, updated_at
		FROM agents 
		WHERE id = $1
	`
	err := pool.QueryRow(ctx, query, agentID).Scan(
		&agent.ID, &agent.Hostname, &agent.IPAddress,
		&agent.OSFamily, &agent.OSVersion, &agent.AgentVersion,
		&agent.LastSeen, &agent.Status, &agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		return nil, ErrAgentNotFound
	}
	return &agent, nil
}
