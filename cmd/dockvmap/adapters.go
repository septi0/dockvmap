package main

import (
	"context"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
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
