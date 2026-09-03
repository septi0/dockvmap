package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) AddAuditLog(ctx context.Context, auditType string, ip, userAgent string, userID int64, username string, data any) error {
	payload, err := json.Marshal(data)

	if err != nil {
		return fmt.Errorf("encoding audit log data: %w", err)
	}

	var userIDArg any

	if userID > 0 {
		userIDArg = userID
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (type, data, ip, user_agent, user_id, username)
		VALUES (?, ?, ?, ?, ?, ?)
	`, auditType, string(payload), ip, userAgent, userIDArg, username)

	if err != nil {
		return fmt.Errorf("adding audit log: %w", err)
	}

	return nil
}

func auditLogListWhere(filters model.AuditLogListFilters) (string, []any) {
	b := &whereBuilder{}

	if filters.Type != "" {
		b.add("type = ?", filters.Type)
	}

	b.dateRange("created_at", filters.Since, filters.Until)

	return b.clause(), b.args
}

func (s *Store) CountAuditLogs(ctx context.Context, filters model.AuditLogListFilters) (int64, error) {
	where, args := auditLogListWhere(filters)

	return s.countWhere(ctx, "audit_logs", where, args)
}

func (s *Store) ListAuditLogs(ctx context.Context, filters model.AuditLogListFilters) ([]model.AuditLog, error) {
	where, args := auditLogListWhere(filters)

	query := fmt.Sprintf(`
		SELECT id, type, data, ip, user_agent, user_id, username, created_at
		FROM audit_logs
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, where)

	rows, err := s.db.QueryContext(ctx, query, append(args, filters.Limit, filters.Offset)...)

	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}

	defer rows.Close()

	logs := make([]model.AuditLog, 0)

	for rows.Next() {
		var (
			entry    model.AuditLog
			data     sql.NullString
			ip       sql.NullString
			ua       sql.NullString
			userID   sql.NullInt64
			username sql.NullString
		)

		if err := rows.Scan(&entry.ID, &entry.Type, &data, &ip, &ua, &userID, &username, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit log: %w", err)
		}

		if data.Valid {
			entry.Data = json.RawMessage(data.String)
		}

		entry.IP = ip.String
		entry.UserAgent = ua.String
		entry.UserID = userID.Int64
		entry.Username = username.String

		logs = append(logs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading audit log rows: %w", err)
	}

	return logs, nil
}
