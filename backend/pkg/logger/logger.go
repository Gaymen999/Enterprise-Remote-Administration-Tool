package logger

import (
	"context"
	"log/slog"
	"os"
	"time"
)

var defaultLogger *slog.Logger

func Init(production bool) {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)

	if production {
		defaultLogger = defaultLogger.With(
			slog.String("environment", "production"),
		)
	}

	slog.SetDefault(defaultLogger)
}

func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
	os.Exit(1)
}

func With(args ...any) *slog.Logger {
	return defaultLogger.With(args...)
}

func Audit(ctx context.Context, action, userID, details string) {
	defaultLogger.Info(action,
		slog.String("user_id", userID),
		slog.String("action", action),
		slog.String("details", details),
		slog.Time("timestamp", time.Now()),
	)
}

func AuditCommand(ctx context.Context, userID, agentID, command string, exitCode int) {
	details := "agent=" + agentID + " command=" + command + " exit_code=" + string(rune(exitCode+'0'))
	Audit(ctx, "command_executed", userID, details)
}

func AuditFileTransfer(ctx context.Context, userID, agentID, operation, path string) {
	details := "agent=" + agentID + " operation=" + operation + " path=" + path
	Audit(ctx, "file_transfer", userID, details)
}

func AuditConnection(ctx context.Context, userID, agentID, event string) {
	details := "agent_id=" + agentID + " event=" + event
	Audit(ctx, "connection_event", userID, details)
}

func AuditAuth(ctx context.Context, userID, event, details string) {
	Audit(ctx, "auth_"+event, userID, details)
}
