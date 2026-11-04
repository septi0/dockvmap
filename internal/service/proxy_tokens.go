package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
)

var ErrInvalidProxyToken = errors.New("invalid proxy token")

const proxyTokenBytes = 32

const maxProxyTokenLabelLength = 100

type proxyTokenStore interface {
	CreateProxyToken(ctx context.Context, label, tokenHash string, canPush bool) (int64, error)
	ListProxyTokens(ctx context.Context) ([]model.ProxyToken, error)
	GetProxyTokenByID(ctx context.Context, id int64) (*model.ProxyToken, error)
	ProxyTokenByHash(ctx context.Context, tokenHash string) (*model.ProxyToken, error)
	DeleteProxyToken(ctx context.Context, id int64) (bool, error)
}

type auditProxyTokenData struct {
	Label   string `json:"label"`
	CanPush bool   `json:"canPush,omitempty"`
}

type tokenUsageRecorder interface {
	RecordTokenUse(tokenID int64)
}

type ProxyTokens struct {
	store proxyTokenStore
	audit auditRecorder
	usage tokenUsageRecorder
}

func NewProxyTokens(store proxyTokenStore, audit auditRecorder, usage tokenUsageRecorder) *ProxyTokens {
	return &ProxyTokens{store: store, audit: audit, usage: usage}
}

func (p *ProxyTokens) Create(ctx context.Context, label string, canPush bool) (int64, string, error) {
	label = strings.TrimSpace(label)

	if label == "" {
		return 0, "", fmt.Errorf("%w: label is required", ErrInvalidProxyToken)
	}

	if len(label) > maxProxyTokenLabelLength {
		return 0, "", fmt.Errorf("%w: label must be at most %d characters", ErrInvalidProxyToken, maxProxyTokenLabelLength)
	}

	token, err := generateProxyToken()

	if err != nil {
		return 0, "", err
	}

	id, err := p.store.CreateProxyToken(ctx, label, hashProxyToken(token), canPush)

	if err != nil {
		return 0, "", err
	}

	recordAudit(ctx, p.audit, AuditTypeProxyTokenCreated, auditProxyTokenData{Label: label, CanPush: canPush})

	return id, token, nil
}

func (p *ProxyTokens) List(ctx context.Context) ([]model.ProxyToken, error) {
	return p.store.ListProxyTokens(ctx)
}

func (p *ProxyTokens) Verify(ctx context.Context, token string) (*model.ProxyToken, error) {
	if token == "" {
		return nil, nil
	}

	matched, err := p.store.ProxyTokenByHash(ctx, hashProxyToken(token))

	if err != nil || matched == nil {
		return nil, err
	}

	p.usage.RecordTokenUse(matched.ID)

	return matched, nil
}

func (p *ProxyTokens) Delete(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("%w: id must be positive", ErrInvalidProxyToken)
	}

	token, err := p.store.GetProxyTokenByID(ctx, id)

	if err != nil {
		return false, err
	}

	if token == nil {
		return false, nil
	}

	deleted, err := p.store.DeleteProxyToken(ctx, id)

	if err != nil {
		return false, err
	}

	if !deleted {
		return false, nil
	}

	recordAudit(ctx, p.audit, AuditTypeProxyTokenDeleted, auditProxyTokenData{Label: token.Label})

	return true, nil
}

func generateProxyToken() (string, error) {
	buf := make([]byte, proxyTokenBytes)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating proxy token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func hashProxyToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
