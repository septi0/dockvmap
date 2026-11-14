package model

import "time"

// ReplicationState, the Replicated* types and RepositoryChange are the replication wire format; changing one changes the API (replicationapi.APIVersion)
type ReplicationState struct {
	ClusterID string `json:"cluster"`
	Version   int64  `json:"version"`
	Tag       string `json:"tag"`
}

type ReplicationSnapshot struct {
	State        ReplicationState       `json:"state"`
	Registries   []ReplicatedRegistry   `json:"registries"`
	Groups       []ReplicatedGroup      `json:"groups"`
	Images       []ReplicatedImage      `json:"images"`
	ProxyTokens  []ReplicatedProxyToken `json:"proxyTokens"`
	Repositories []ReplicatedRepository `json:"repositories"`
}

type ReplicatedRegistry struct {
	ID         int64           `json:"id"`
	Registry   string          `json:"registry"`
	Username   string          `json:"username,omitempty"`
	Credential string          `json:"credential,omitempty"`
	Options    RegistryOptions `json:"options"`
	Internal   bool            `json:"internal"`
}

type ReplicatedGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ReplicatedImage struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	RegistryID int64  `json:"registryId"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Pinned     bool   `json:"pinned"`
	GroupID    *int64 `json:"groupId,omitempty"`
}

type ReplicatedProxyToken struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	TokenHash string    `json:"tokenHash"`
	CanPush   bool      `json:"canPush"`
	CreatedAt time.Time `json:"createdAt"`
}

type ReplicatedRepository struct {
	Repository string `json:"repository"`
	Counter    int64  `json:"counter"`
}

type RepositoryChange struct {
	Reference string `json:"reference"`
	Deleted   bool   `json:"deleted"`
	Counter   int64  `json:"counter"`
}

type ReplicationToken struct {
	ID         int64
	Label      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

type FollowerState struct {
	ClusterID       string
	AppliedVersion  int64
	AppliedTag      string
	AppliedAt       *time.Time
	InitialCopyDone bool
}

type LocalInternalRegistry struct {
	Address string
	Options RegistryOptions
}
