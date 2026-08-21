package main

import (
	"context"

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
