package replicationapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
)

const APIVersion = 1

const (
	StatePath      = "/replication/v1/state"
	ChangesPath    = "/replication/v1/changes"
	PullsPath      = "/replication/v1/pulls"
	RegistryPrefix = "/replication/v1/registry"
)

const FollowerStatusHeader = "X-Dockvmap-Follower"

const (
	ParamVersion = "version"
	ParamTag     = "tag"
	ParamCluster = "cluster"
	ParamFull    = "full"

	ParamRepository = "repository"
	ParamAfter      = "after"
)

const (
	ConflictDiverged     = "diverged"
	ConflictOtherCluster = "other-cluster"
)

type StateResponse struct {
	APIVersion int                       `json:"apiVersion"`
	Setup      Setup                     `json:"setup"`
	Snapshot   model.ReplicationSnapshot `json:"snapshot"`
}

// Complete is false when some changes were pruned, so the follower compares the whole repository instead
type ChangesResponse struct {
	Changes  []model.RepositoryChange `json:"changes"`
	Complete bool                     `json:"complete"`
}

// PullReport carries a follower's latest pulls and proxy token uses; the primary keeps the newer time of each
type PullReport struct {
	Images []model.ImageUsage      `json:"images"`
	Tokens []model.ProxyTokenUsage `json:"tokens"`
}

type Conflict struct {
	Reason         string `json:"reason"`
	PrimaryVersion int64  `json:"primaryVersion"`
}

type Error struct {
	Error string `json:"error"`
}

// Setup holds the settings a follower must share with its primary, without secrets; only settings both sides send are compared
type Setup map[string]string

type Mismatch struct {
	Setting  string `json:"setting"`
	Primary  string `json:"primary"`
	Follower string `json:"follower"`
}

func (s Setup) Mismatches(primary Setup) []Mismatch {
	var mismatches []Mismatch

	for _, key := range slices.Sorted(maps.Keys(s)) {
		if value, ok := primary[key]; ok && value != s[key] {
			mismatches = append(mismatches, Mismatch{Setting: key, Primary: primary[key], Follower: s[key]})
		}
	}

	return mismatches
}

var (
	ErrIdentityChanged = errors.New("the primary's identity doesn't match follower.token; if its replication.pem was replaced, create a new replication token on the primary and update follower.token")
	ErrTokenRefused    = errors.New("the primary refused follower.token; it was deleted or belongs to another primary, so create a new one on the primary's System > Replication page")
	ErrNewerAPI        = errors.New("the primary uses a newer replication API; update this follower to the primary's DockVMap version")
)

const tokenPrefix = "dvmr1"

var ErrInvalidToken = errors.New("follower.token isn't a replication token; copy it again from the primary's System > Replication page")

func NewToken(fingerprint string) (token, secret string, err error) {
	raw := make([]byte, 32)

	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	secret = hex.EncodeToString(raw)

	return tokenPrefix + "-" + strings.TrimPrefix(fingerprint, "sha256:") + "-" + secret, secret, nil
}

func ParseToken(token string) (fingerprint, secret string, err error) {
	parts := strings.Split(strings.TrimSpace(token), "-")

	if len(parts) != 3 || parts[0] != tokenPrefix || !isHex(parts[1], 64) || !isHex(parts[2], 64) {
		return "", "", ErrInvalidToken
	}

	return "sha256:" + parts[1], parts[2], nil
}

func isHex(value string, length int) bool {
	_, err := hex.DecodeString(value)

	return err == nil && len(value) == length && value == strings.ToLower(value)
}
