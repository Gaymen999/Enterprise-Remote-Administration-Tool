-- Idempotent Seed Data for Development
-- Migration: 002_seed_data
-- Run this script multiple times - it will reset all data each time

-- Reset all data (idempotent - safe to run multiple times)
TRUNCATE TABLE audit_logs, command_results, commands, agents, users CASCADE;

-- ============================================
-- Mock Admin User (password: SecureP@ss123!)
-- Valid bcrypt hash for "SecureP@ss123!" (cost 10)
-- IMPORTANT: Change this password before production deployment
-- ============================================
INSERT INTO users (id, username, password_hash, role, is_active, created_at, updated_at)
VALUES
    (
        'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
        'admin',
        '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
        'admin',
        true,
        NOW(),
        NOW()
    ),
    (
        'b2c3d4e5-f6a7-8901-bcde-f23456789012',
        'operator',
        '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
        'operator',
        true,
        NOW(),
        NOW()
    );

-- ============================================
-- Mock Agents
-- ============================================

INSERT INTO agents (id, hostname, ip_address, os_family, os_version, agent_version, last_seen, status, created_at, updated_at)
VALUES
    (
        '11111111-1111-1111-1111-111111111111',
        'WIN-SRV-PROD-01',
        '192.168.1.101',
        'windows',
        'Windows Server 2022',
        '1.0.0',
        NOW(),
        'online',
        NOW() - INTERVAL '7 days',
        NOW()
    ),
    (
        '22222222-2222-2222-2222-222222222222',
        'ubuntu-app-prod-01',
        '192.168.1.102',
        'linux',
        'Ubuntu 22.04 LTS',
        '1.0.0',
        NOW() - INTERVAL '2 hours',
        'offline',
        NOW() - INTERVAL '14 days',
        NOW() - INTERVAL '2 hours'
    ),
    (
        '33333333-3333-3333-3333-333333333333',
        'MACBOOK-PRO-DESIGN-03',
        '192.168.1.103',
        'darwin',
        'macOS Sonoma 14.5',
        '1.0.0',
        NOW(),
        'online',
        NOW() - INTERVAL '3 days',
        NOW()
    );

-- ============================================
-- Sample Commands (JSON format for command_payload)
-- ============================================

INSERT INTO commands (id, agent_id, user_id, command_payload, command_args, command_timeout, status, created_at, updated_at)
VALUES
    (
        'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
        '11111111-1111-1111-1111-111111111111',
        'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
        '{"executable": "systeminfo"}',
        '[]'::jsonb,
        300,
        'completed',
        NOW() - INTERVAL '1 hour',
        NOW() - INTERVAL '1 hour'
    ),
    (
        'cccccccc-cccc-cccc-cccc-cccccccccccc',
        '33333333-3333-3333-3333-333333333333',
        'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
        '{"executable": "df", "args": ["-h"]}',
        '["-h"]'::jsonb,
        300,
        'completed',
        NOW() - INTERVAL '30 minutes',
        NOW() - INTERVAL '30 minutes'
    );

-- ============================================
-- Command Results
-- ============================================

INSERT INTO command_results (id, command_id, stdout, stderr, exit_code, completed_at)
VALUES
    (
        'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
        'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
        'Host Name:           WIN-SRV-PROD-01
OS Name:             Microsoft Windows Server 2022 Standard
OS Version:          10.0.20348 N/A Build 20348
System Type:         x64-based PC
Total Physical Memory: 16,383 MB',
        '',
        0,
        NOW() - INTERVAL '1 hour'
    ),
    (
        'dddddddd-dddd-dddd-dddd-dddddddddddd',
        'cccccccc-cccc-cccc-cccc-cccccccccccc',
        'Filesystem      Size  Used Avail Use% Mounted on
/dev/disk1s1   466Gi 256Gi 210Gi  55% /
/dev/disk1s4   5.4Gi  14Mi  5.4Gi   1% /private/var/vm',
        '',
        0,
        NOW() - INTERVAL '30 minutes'
    );

-- ============================================
-- Sample Audit Logs
-- ============================================

INSERT INTO audit_logs (id, user_id, action, resource_type, resource_id, details, ip_address, created_at)
VALUES
    (
        'eeeeeee1-1111-1111-1111-111111111111',
        'a1b2c3d4-e5f6-7890-abcd-ef1234567890',
        'user.login',
        'session',
        NULL,
        '{"method": "password"}',
        '192.168.1.50',
        NOW() - INTERVAL '5 minutes'
    );

-- Verify data was inserted
SELECT 'Users:' as info, COUNT(*) as count FROM users
UNION ALL
SELECT 'Agents:', COUNT(*) FROM agents
UNION ALL
SELECT 'Commands:', COUNT(*) FROM commands;
