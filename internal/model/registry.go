package model

type Registry struct {
	ID         int64           `json:"id"`
	Registry   string          `json:"registry"`
	Username   string          `json:"username,omitempty"`
	Credential string          `json:"-"`
	Options    RegistryOptions `json:"options"`
}

type RegistryUpdate struct {
	ID         int64
	Registry   string
	Username   *string
	Credential *string
	Options    *RegistryOptions
}

type RegistryCredentials struct {
	Username   string
	Credential string
}

type RegistryOptions struct {
	Insecure             bool `json:"insecure"`
	AllowSelfSignedCerts bool `json:"allow_self_signed_certs"`
}

type RegistryInfo struct {
	ID                       int64           `json:"id"`
	Registry                 string          `json:"registry"`
	Username                 string          `json:"username,omitempty"`
	AuthenticationConfigured bool            `json:"authenticationConfigured"`
	Options                  RegistryOptions `json:"options"`
}
