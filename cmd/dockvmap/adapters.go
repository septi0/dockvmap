package main

import (
	"context"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/ocicopy"
	"github.com/septi0/dockvmap/internal/service"
)

type registryCredentialsAdapter struct {
	registries *service.Registries
}

func (a registryCredentialsAdapter) GetRegistryCredentials(ctx context.Context, registryHost string) (*oci.Credentials, error) {
	credentials, err := a.registries.GetRegistryCredentials(ctx, registryHost)

	if err != nil {
		return nil, err
	}

	if credentials == nil || credentials.Username == "" || credentials.Credential == "" {
		return nil, nil
	}

	return &oci.Credentials{
		Username:   credentials.Username,
		Credential: credentials.Credential,
	}, nil
}

type registryConnCheckerAdapter struct{}

func (registryConnCheckerAdapter) CheckRegistryConnection(ctx context.Context, host string, creds *model.RegistryCredentials, opts model.RegistryOptions) error {
	var ociCreds *oci.Credentials

	if creds != nil && creds.Username != "" && creds.Credential != "" {
		ociCreds = &oci.Credentials{Username: creds.Username, Credential: creds.Credential}
	}

	client := oci.NewClient(nil,
		staticRegistryCredentials{creds: ociCreds},
		staticRegistryOptions{opts: &oci.RegistryOptions{
			Insecure:             opts.Insecure,
			AllowSelfSignedCerts: opts.AllowSelfSignedCerts,
		}},
	)

	return client.CheckRegistry(ctx, host)
}

type staticRegistryCredentials struct {
	creds *oci.Credentials
}

func (s staticRegistryCredentials) GetRegistryCredentials(context.Context, string) (*oci.Credentials, error) {
	return s.creds, nil
}

type staticRegistryOptions struct {
	opts *oci.RegistryOptions
}

func (s staticRegistryOptions) GetRegistryOptions(context.Context, string) (*oci.RegistryOptions, error) {
	return s.opts, nil
}

type registryOptionsAdapter struct {
	registries *service.Registries
}

func (a registryOptionsAdapter) GetRegistryOptions(ctx context.Context, registryHost string) (*oci.RegistryOptions, error) {
	registryOptions, err := a.registries.GetRegistryOptions(ctx, registryHost)

	if err != nil {
		return nil, err
	}

	if registryOptions == nil {
		return &oci.RegistryOptions{}, nil
	}

	return &oci.RegistryOptions{
		Insecure:             registryOptions.Insecure,
		AllowSelfSignedCerts: registryOptions.AllowSelfSignedCerts,
	}, nil
}

type followerMirror struct {
	primary ocicopy.Registry
	local   ocicopy.Registry
}

func (m followerMirror) Repositories(ctx context.Context) ([]string, error) {
	return m.primary.Repositories(ctx)
}

func (m followerMirror) Tags(ctx context.Context, repository string) ([]string, error) {
	return m.primary.Tags(ctx, repository)
}

func (m followerMirror) Copy(ctx context.Context, repository, tag string, deep bool) (bool, error) {
	return ocicopy.Copy(ctx, m.primary, m.local, repository, tag, deep)
}

func (m followerMirror) HasTag(ctx context.Context, repository, tag string) (bool, error) {
	digest, err := m.local.ManifestDigest(ctx, repository, tag)

	return digest != "", err
}

func (m followerMirror) Delete(ctx context.Context, repository, reference string) error {
	return m.local.Delete(ctx, repository, reference)
}

func (m followerMirror) LocalTags(ctx context.Context) (map[string][]string, error) {
	repositories, err := m.local.Repositories(ctx)

	if err != nil {
		return nil, err
	}

	tags := make(map[string][]string, len(repositories))

	for _, repository := range repositories {
		if tags[repository], err = m.local.Tags(ctx, repository); err != nil {
			return nil, err
		}
	}

	return tags, nil
}

func (m followerMirror) DeleteTags(ctx context.Context, repository string, tags []string) error {
	return m.local.DeleteTags(ctx, repository, tags)
}

func (m followerMirror) DeletesAllowed(ctx context.Context) (bool, error) {
	return m.local.DeletesAllowed(ctx)
}
