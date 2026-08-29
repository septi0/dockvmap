package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

var (
	ErrInvalidRegistry                   = errors.New("invalid registry")
	ErrRegistryAlreadyExists             = errors.New("registry already exists")
	ErrRegistryInUse                     = errors.New("registry is in use")
	ErrCredentialEncryptionNotConfigured = errors.New("registry credential encryption is not configured")
)

type registryStore interface {
	GetRegistryCredentials(ctx context.Context, registry string) (*model.RegistryCredentials, error)
	GetRegistryInfoByID(ctx context.Context, registryID int64) (*model.RegistryInfo, error)
	ListRegistryInfo(ctx context.Context) ([]model.RegistryInfo, error)
	GetRegistryOptions(ctx context.Context, registry string) (*model.RegistryOptions, error)
	CreateRegistry(ctx context.Context, registry model.Registry) (int64, error)
	UpdateRegistryByID(ctx context.Context, registry model.RegistryUpdate) (bool, error)
	DeleteRegistryByID(ctx context.Context, registryID int64) (bool, error)
}

type Registries struct {
	store registryStore
	audit auditRecorder
}

type auditRegistryData struct {
	Registry                 string                `json:"registry"`
	Options                  model.RegistryOptions `json:"options"`
	AuthenticationConfigured bool                  `json:"authenticationConfigured"`
}

type auditRegistryUpdateData struct {
	auditRegistryData
	CredentialChanged bool `json:"credentialChanged"`
}

func NewRegistries(store registryStore, audit auditRecorder) *Registries {
	return &Registries{store: store, audit: audit}
}

func (r *Registries) Create(ctx context.Context, registry model.Registry) (int64, error) {
	registry.Registry = strings.TrimSpace(registry.Registry)
	registry.Username = strings.TrimSpace(registry.Username)

	if !validRegistryAddress(registry.Registry) {
		return 0, fmt.Errorf("%w: registry must be a valid host", ErrInvalidRegistry)
	}

	if (registry.Username != "" || registry.Credential != "") &&
		(registry.Username == "" || registry.Credential == "") {
		return 0, fmt.Errorf(
			"%w: username and credential must both be provided together when authentication is configured",
			ErrInvalidRegistry,
		)
	}

	registryID, err := r.store.CreateRegistry(ctx, registry)

	if errors.Is(err, store.ErrRegistryConflict) {
		return 0, ErrRegistryAlreadyExists
	}

	if errors.Is(err, store.ErrCredentialEncryptionNotConfigured) {
		return 0, ErrCredentialEncryptionNotConfigured
	}

	if err != nil {
		return 0, err
	}

	recordAudit(ctx, r.audit, AuditTypeRegistryCreated, auditRegistryData{
		Registry:                 registry.Registry,
		Options:                  registry.Options,
		AuthenticationConfigured: registry.Username != "",
	})

	return registryID, nil
}

func (r *Registries) List(ctx context.Context) ([]model.RegistryInfo, error) {
	return r.store.ListRegistryInfo(ctx)
}

func (r *Registries) GetRegistryCredentials(ctx context.Context, registry string) (*model.RegistryCredentials, error) {
	return r.store.GetRegistryCredentials(ctx, registry)
}

func (r *Registries) GetRegistryOptions(ctx context.Context, registry string) (*model.RegistryOptions, error) {
	return r.store.GetRegistryOptions(ctx, registry)
}

func (r *Registries) GetByID(ctx context.Context, registryID int64) (*model.RegistryInfo, error) {
	if registryID <= 0 {
		return nil, fmt.Errorf("%w: registry id must be greater than zero", ErrInvalidRegistry)
	}

	return r.store.GetRegistryInfoByID(ctx, registryID)
}

func (r *Registries) UpdateByID(ctx context.Context, registry model.RegistryUpdate) (bool, error) {
	if registry.ID <= 0 {
		return false, fmt.Errorf("%w: registry id must be greater than zero", ErrInvalidRegistry)
	}

	registry.Registry = strings.TrimSpace(registry.Registry)

	if !validRegistryAddress(registry.Registry) {
		return false, fmt.Errorf("%w: registry must be a valid host", ErrInvalidRegistry)
	}

	if (registry.Username == nil) != (registry.Credential == nil) {
		return false, fmt.Errorf("%w: username and credential must both be provided together when changing authentication", ErrInvalidRegistry)
	}

	if registry.Username != nil {
		*registry.Username = strings.TrimSpace(*registry.Username)
	}

	if registry.Credential != nil {
		if *registry.Username == "" && *registry.Credential != "" {
			return false, fmt.Errorf("%w: username is required when configuring authentication", ErrInvalidRegistry)
		}

		if *registry.Username != "" && *registry.Credential == "" {
			return false, fmt.Errorf("%w: credential is required when configuring authentication", ErrInvalidRegistry)
		}
	}

	updated, err := r.store.UpdateRegistryByID(ctx, registry)

	if errors.Is(err, store.ErrRegistryConflict) {
		return false, ErrRegistryAlreadyExists
	}

	if errors.Is(err, store.ErrCredentialEncryptionNotConfigured) {
		return false, ErrCredentialEncryptionNotConfigured
	}

	if err != nil || !updated {
		return updated, err
	}

	info, infoErr := r.store.GetRegistryInfoByID(ctx, registry.ID)

	if infoErr != nil || info == nil {
		slog.Error("failed to record audit log", "type", AuditTypeRegistryUpdated, "error", infoErr)
	} else {
		recordAudit(ctx, r.audit, AuditTypeRegistryUpdated, auditRegistryUpdateData{
			auditRegistryData: auditRegistryData{
				Registry:                 info.Registry,
				Options:                  info.Options,
				AuthenticationConfigured: info.AuthenticationConfigured,
			},
			CredentialChanged: registry.Credential != nil,
		})
	}

	return true, nil
}

func (r *Registries) DeleteByID(ctx context.Context, registryID int64) (bool, error) {
	if registryID <= 0 {
		return false, fmt.Errorf("%w: registry id must be greater than zero", ErrInvalidRegistry)
	}

	info, err := r.store.GetRegistryInfoByID(ctx, registryID)

	if err != nil {
		return false, err
	}

	deleted, err := r.store.DeleteRegistryByID(ctx, registryID)

	if errors.Is(err, store.ErrRegistryInUse) {
		return false, ErrRegistryInUse
	}

	if err != nil || !deleted {
		return deleted, err
	}

	if info != nil {
		recordAudit(ctx, r.audit, AuditTypeRegistryDeleted, auditRegistryData{
			Registry:                 info.Registry,
			Options:                  info.Options,
			AuthenticationConfigured: info.AuthenticationConfigured,
		})
	}

	return true, nil
}

func (r *Registries) Get(ctx context.Context, registryID int64) (*model.RegistryInfo, error) {
	return r.GetByID(ctx, registryID)
}

func (r *Registries) Update(ctx context.Context, registry model.RegistryUpdate) (bool, error) {
	return r.UpdateByID(ctx, registry)
}

func (r *Registries) Delete(ctx context.Context, registryID int64) (bool, error) {
	return r.DeleteByID(ctx, registryID)
}
