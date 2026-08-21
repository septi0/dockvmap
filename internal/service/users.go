package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUser              = errors.New("invalid user")
	ErrUsernameConflict         = errors.New("username already exists")
	ErrUserNotFound             = errors.New("user not found")
	ErrSetupComplete            = errors.New("initial setup has already been completed")
	ErrIncorrectCurrentPassword = errors.New("current password is incorrect")
)

var (
	usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)
	emailRE    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

const (
	minPasswordLength = 8
	maxPasswordLength = 72
)

type userStore interface {
	CountUsers(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByID(ctx context.Context, userID int64) (*model.User, error)
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) (bool, error)
	UpdateUserEmail(ctx context.Context, userID int64, email string) (bool, error)
	UpdateUserPreferences(ctx context.Context, userID int64, preferences model.UserPreferences) (bool, error)
}

type auditUserData struct {
	Username string `json:"username"`
}

type sessionInvalidator interface {
	InvalidateOtherSessions(ctx context.Context, userID int64, exceptToken string) error
}

type Users struct {
	store          userStore
	audit          auditRecorder
	sessions       sessionInvalidator
	bootstrapMutex sync.Mutex
}

func NewUsers(store userStore, audit auditRecorder, sessions sessionInvalidator) *Users {
	return &Users{store: store, audit: audit, sessions: sessions}
}

func (u *Users) SetupRequired(ctx context.Context) (bool, error) {
	count, err := u.store.CountUsers(ctx)

	if err != nil {
		return false, err
	}

	return count == 0, nil
}

func (u *Users) Bootstrap(ctx context.Context, username, email, password string) (int64, error) {
	u.bootstrapMutex.Lock()
	defer u.bootstrapMutex.Unlock()

	count, err := u.store.CountUsers(ctx)

	if err != nil {
		return 0, err
	}

	if count > 0 {
		return 0, ErrSetupComplete
	}

	user, err := u.createUser(ctx, username, email, password)

	if err != nil {
		return 0, err
	}

	recordAudit(ctx, u.audit, AuditTypeUserBootstrapped, auditUserData{Username: user.Username})

	return user.ID, nil
}

func (u *Users) createUser(ctx context.Context, username, email, password string) (*model.User, error) {
	username = strings.TrimSpace(username)

	if !usernameRE.MatchString(username) {
		return nil, fmt.Errorf("%w: username must be 3-64 characters (letters, digits, dots, underscores, hyphens)", ErrInvalidUser)
	}

	email, err := normalizeEmail(email)

	if err != nil {
		return nil, err
	}

	hash, err := hashPassword(password)

	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		Preferences:  model.UserPreferences{NotifyNewTags: true},
	}

	if err := u.store.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrUsernameConflict) {
			return nil, ErrUsernameConflict
		}

		return nil, err
	}

	return user, nil
}

func (u *Users) ResetPassword(ctx context.Context, username string) (string, error) {
	username = strings.TrimSpace(username)

	user, err := u.store.GetUserByUsername(ctx, username)

	if err != nil {
		return "", err
	}

	if user == nil {
		return "", fmt.Errorf("%w: %q", ErrUserNotFound, username)
	}

	password, err := generateRandomPassword()

	if err != nil {
		return "", err
	}

	hash, err := hashPassword(password)

	if err != nil {
		return "", err
	}

	updated, err := u.store.UpdateUserPassword(ctx, user.ID, hash)

	if err != nil {
		return "", err
	}

	if !updated {
		return "", fmt.Errorf("%w: %d", ErrUserNotFound, user.ID)
	}

	if err := u.sessions.InvalidateOtherSessions(ctx, user.ID, ""); err != nil {
		slog.Error("failed to invalidate sessions after password reset", "error", err)
	}

	recordAudit(ctx, u.audit, AuditTypeUserPasswordReset, auditUserData{Username: user.Username})

	return password, nil
}

func generateRandomPassword() (string, error) {
	buf := make([]byte, 18)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)

	if !emailRE.MatchString(email) {
		return "", fmt.Errorf("%w: email must be a valid email address", ErrInvalidUser)
	}

	return email, nil
}

func (u *Users) UpdatePassword(ctx context.Context, currentPassword, newPassword string) error {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	user, err := u.store.GetUserByID(ctx, currentUser.ID)

	if err != nil {
		return err
	}

	if user == nil {
		return fmt.Errorf("%w: %d", ErrUserNotFound, currentUser.ID)
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		recordAudit(ctx, u.audit, AuditTypeUserPasswordChangeFailed, nil)

		return ErrIncorrectCurrentPassword
	}

	hash, err := hashPassword(newPassword)

	if err != nil {
		return err
	}

	updated, err := u.store.UpdateUserPassword(ctx, user.ID, hash)

	if err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrUserNotFound, user.ID)
	}

	if err := u.sessions.InvalidateOtherSessions(ctx, user.ID, SessionTokenFromContext(ctx)); err != nil {
		slog.Error("failed to invalidate other sessions after password change", "error", err)
	}

	recordAudit(ctx, u.audit, AuditTypeUserPasswordChanged, nil)

	return nil
}

func (u *Users) GetProfile(ctx context.Context) (*model.User, error) {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return nil, fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	user, err := u.store.GetUserByID(ctx, currentUser.ID)

	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, fmt.Errorf("%w: %d", ErrUserNotFound, currentUser.ID)
	}

	return user, nil
}

func (u *Users) UpdateEmail(ctx context.Context, email string) error {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	email, err := normalizeEmail(email)

	if err != nil {
		return err
	}

	updated, err := u.store.UpdateUserEmail(ctx, currentUser.ID, email)

	if err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrUserNotFound, currentUser.ID)
	}

	recordAudit(ctx, u.audit, AuditTypeUserEmailChanged, nil)

	return nil
}

func (u *Users) UpdatePreferences(ctx context.Context, patch model.UserPreferencesUpdate) error {
	currentUser, ok := CurrentUserFromContext(ctx)

	if !ok {
		return fmt.Errorf("%w: no authenticated user", ErrUserNotFound)
	}

	user, err := u.store.GetUserByID(ctx, currentUser.ID)

	if err != nil {
		return err
	}

	if user == nil {
		return fmt.Errorf("%w: %d", ErrUserNotFound, currentUser.ID)
	}

	preferences := user.Preferences

	if patch.NotifyNewTags != nil {
		preferences.NotifyNewTags = *patch.NotifyNewTags
	}

	updated, err := u.store.UpdateUserPreferences(ctx, currentUser.ID, preferences)

	if err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrUserNotFound, currentUser.ID)
	}

	return nil
}

func hashPassword(password string) (string, error) {
	if len(password) < minPasswordLength {
		return "", fmt.Errorf("%w: password must be at least %d characters", ErrInvalidUser, minPasswordLength)
	}

	if len(password) > maxPasswordLength {
		return "", fmt.Errorf("%w: password must be at most %d characters", ErrInvalidUser, maxPasswordLength)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}

	return string(hash), nil
}
