package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/septi0/dockvmap/internal/model"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrSessionNotFound    = errors.New("session not found")
	ErrLoginRateLimited   = errors.New("too many failed login attempts")
)

const sessionTokenBytes = 32

const dummyPasswordHash = "$2a$10$pA9QmTH9/RNf3MQmpHcSIOCnzy4RNuavb14ikGWa.79lIu/2md1KG"

type auditSessionData struct {
	Username string `json:"username"`
}

type sessionStore interface {
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	CreateSession(ctx context.Context, token string, userID int64, ip, userAgent string, expiresAt time.Time) error
	GetSessionUser(ctx context.Context, token string) (*model.CurrentUser, error)
	ListSessionsByUser(ctx context.Context, userID int64, currentToken string) ([]model.Session, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteSessionByID(ctx context.Context, userID, sessionID int64, exceptToken string) (bool, error)
	DeleteExpiredSessions(ctx context.Context) (int64, error)
	DeleteOtherSessions(ctx context.Context, userID int64, exceptToken string) error
}

type loginLimiter interface {
	Reserve(ip string) bool
	RecordSuccess(ip string)
}

type Sessions struct {
	store       sessionStore
	lifetime    time.Duration
	audit       auditRecorder
	rateLimiter loginLimiter
}

func NewSessions(store sessionStore, lifetime time.Duration, audit auditRecorder, rateLimiter loginLimiter) *Sessions {
	return &Sessions{store: store, lifetime: lifetime, audit: audit, rateLimiter: rateLimiter}
}

func (s *Sessions) Login(ctx context.Context, username, password string) (string, time.Time, error) {
	info := requestInfoFromContext(ctx)

	if !s.rateLimiter.Reserve(info.IP) {
		slog.Warn("login blocked by rate limiter", "ip", info.IP, "username", username)

		return "", time.Time{}, ErrLoginRateLimited
	}

	user, err := s.store.GetUserByUsername(ctx, username)

	if err != nil {
		return "", time.Time{}, err
	}

	hash := dummyPasswordHash

	if user != nil {
		hash = user.PasswordHash
	}

	compareErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))

	if user == nil || compareErr != nil {
		recordAudit(ctx, s.audit, AuditTypeUserLoginFailed, auditSessionData{Username: username})

		return "", time.Time{}, ErrInvalidCredentials
	}

	s.rateLimiter.RecordSuccess(info.IP)

	token, err := generateSessionToken()

	if err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().UTC().Add(s.lifetime)

	if err := s.store.CreateSession(ctx, token, user.ID, info.IP, info.UserAgent, expiresAt); err != nil {
		return "", time.Time{}, err
	}

	if _, err := s.store.DeleteExpiredSessions(ctx); err != nil {
		slog.Warn("failed to clean up expired sessions", "error", err)
	}

	recordAudit(WithCurrentUser(ctx, model.CurrentUser{ID: user.ID, Username: user.Username}), s.audit, AuditTypeUserLoggedIn, nil)

	return token, expiresAt, nil
}

func (s *Sessions) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	user, err := s.store.GetSessionUser(ctx, token)

	if err != nil {
		return err
	}

	if err := s.store.DeleteSession(ctx, token); err != nil {
		return err
	}

	if user != nil {
		recordAudit(WithCurrentUser(ctx, *user), s.audit, AuditTypeUserLoggedOut, nil)
	}

	return nil
}

func (s *Sessions) Validate(ctx context.Context, token string) (*model.CurrentUser, error) {
	if token == "" {
		return nil, nil
	}

	return s.store.GetSessionUser(ctx, token)
}

func (s *Sessions) InvalidateOtherSessions(ctx context.Context, userID int64, exceptToken string) error {
	return s.store.DeleteOtherSessions(ctx, userID, exceptToken)
}

func (s *Sessions) CleanupExpired(ctx context.Context) (int64, error) {
	return s.store.DeleteExpiredSessions(ctx)
}

func (s *Sessions) ListActive(ctx context.Context) ([]model.Session, error) {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return nil, fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	return s.store.ListSessionsByUser(ctx, currentUser.ID, SessionTokenFromContext(ctx))
}

func (s *Sessions) InvalidateSession(ctx context.Context, sessionID int64) error {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	deleted, err := s.store.DeleteSessionByID(ctx, currentUser.ID, sessionID, SessionTokenFromContext(ctx))

	if err != nil {
		return err
	}

	if !deleted {
		return ErrSessionNotFound
	}

	recordAudit(ctx, s.audit, AuditTypeUserSessionRevoked, nil)

	return nil
}

func generateSessionToken() (string, error) {
	buf := make([]byte, sessionTokenBytes)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}
