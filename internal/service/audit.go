package service

import (
	"context"
	"log/slog"
	"slices"

	"github.com/septi0/dockvmap/internal/model"
)

const (
	AuditTypeRegistryCreated = "REGISTRY_CREATED"
	AuditTypeRegistryUpdated = "REGISTRY_UPDATED"
	AuditTypeRegistryDeleted = "REGISTRY_DELETED"
	AuditTypeImageCreated    = "IMAGE_CREATED"
	AuditTypeImageTagChanged = "IMAGE_TAG_CHANGED"
	AuditTypeImageDeleted    = "IMAGE_DELETED"

	AuditTypeUserBootstrapped         = "USER_BOOTSTRAPPED"
	AuditTypeUserCreated              = "USER_CREATED"
	AuditTypeUserPasswordChanged      = "USER_PASSWORD_CHANGED"
	AuditTypeUserPasswordChangeFailed = "USER_PASSWORD_CHANGE_FAILED"
	AuditTypeUserPasswordReset        = "USER_PASSWORD_RESET"
	AuditTypeUserEmailChanged         = "USER_EMAIL_CHANGED"
	AuditTypeUserDeleted              = "USER_DELETED"

	AuditTypeUserLoggedIn       = "USER_LOGGED_IN"
	AuditTypeUserLoginFailed    = "USER_LOGIN_FAILED"
	AuditTypeUserLoggedOut      = "USER_LOGGED_OUT"
	AuditTypeUserSessionRevoked = "USER_SESSION_REVOKED"

	AuditTypeProxyTokenCreated = "PROXY_TOKEN_CREATED"
	AuditTypeProxyTokenDeleted = "PROXY_TOKEN_DELETED"
)

var AuditTypes = []string{
	AuditTypeRegistryCreated,
	AuditTypeRegistryUpdated,
	AuditTypeRegistryDeleted,
	AuditTypeImageCreated,
	AuditTypeImageTagChanged,
	AuditTypeImageDeleted,

	AuditTypeUserBootstrapped,
	AuditTypeUserCreated,
	AuditTypeUserPasswordChanged,
	AuditTypeUserPasswordChangeFailed,
	AuditTypeUserPasswordReset,
	AuditTypeUserEmailChanged,
	AuditTypeUserDeleted,

	AuditTypeUserLoggedIn,
	AuditTypeUserLoginFailed,
	AuditTypeUserLoggedOut,
	AuditTypeUserSessionRevoked,

	AuditTypeProxyTokenCreated,
	AuditTypeProxyTokenDeleted,
}

func IsValidAuditType(t string) bool {
	return slices.Contains(AuditTypes, t)
}

type RequestInfo struct {
	IP        string
	UserAgent string
}

type requestInfoContextKey struct{}

func WithRequestInfo(ctx context.Context, info RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoContextKey{}, info)
}

func requestInfoFromContext(ctx context.Context) RequestInfo {
	info, _ := ctx.Value(requestInfoContextKey{}).(RequestInfo)
	return info
}

type currentUserContextKey struct{}

func WithCurrentUser(ctx context.Context, user model.CurrentUser) context.Context {
	return context.WithValue(ctx, currentUserContextKey{}, user)
}

func CurrentUserFromContext(ctx context.Context) (model.CurrentUser, bool) {
	user, ok := ctx.Value(currentUserContextKey{}).(model.CurrentUser)
	return user, ok
}

type sessionTokenContextKey struct{}

func WithSessionToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, sessionTokenContextKey{}, token)
}

func SessionTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(sessionTokenContextKey{}).(string)
	return token
}

type auditStore interface {
	AddAuditLog(ctx context.Context, auditType string, ip, userAgent string, userID int64, username string, data any) error
	ListAuditLogs(ctx context.Context, filters model.AuditLogListFilters) ([]model.AuditLog, error)
	CountAuditLogs(ctx context.Context, filters model.AuditLogListFilters) (int64, error)
}

type auditRecorder interface {
	Record(ctx context.Context, auditType string, data any) error
}

type Audit struct {
	store auditStore
}

func NewAudit(store auditStore) *Audit {
	return &Audit{store: store}
}

func (a *Audit) Record(ctx context.Context, auditType string, data any) error {
	info := requestInfoFromContext(ctx)
	user, _ := CurrentUserFromContext(ctx)

	return a.store.AddAuditLog(ctx, auditType, info.IP, info.UserAgent, user.ID, user.Username, data)
}

func (a *Audit) List(ctx context.Context, filters model.AuditLogListFilters) ([]model.AuditLog, error) {
	return a.store.ListAuditLogs(ctx, filters)
}

func (a *Audit) Count(ctx context.Context, filters model.AuditLogListFilters) (int64, error) {
	return a.store.CountAuditLogs(ctx, filters)
}

func recordAudit(ctx context.Context, audit auditRecorder, auditType string, data any) {
	if audit == nil {
		return
	}

	if err := audit.Record(ctx, auditType, data); err != nil {
		slog.Error("failed to record audit log", "type", auditType, "error", err)
	}
}
