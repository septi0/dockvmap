package agent

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const (
	identityFile = "agent.pem"
	pairingFile  = "paired.json"
)

type pairing struct {
	DockVMap string    `json:"dockvmap"`
	PairedAt time.Time `json:"pairedAt"`
}

func loadIdentity(dataPath string) (tls.Certificate, error) {
	path := filepath.Join(dataPath, identityFile)

	data, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		if data, err = agentapi.NewIdentity("dockvmap-agent"); err != nil {
			return tls.Certificate{}, fmt.Errorf("creating the agent identity: %w", err)
		}

		if err := writeFileAtomic(path, data, 0o600); err != nil {
			return tls.Certificate{}, fmt.Errorf("saving the agent identity in %s: %w", dataPath, err)
		}

		slog.Info("created the agent identity", "path", path)
	} else if err != nil {
		return tls.Certificate{}, fmt.Errorf("reading the agent identity: %w", err)
	}

	identity, err := agentapi.ParseIdentity(data)

	if err != nil {
		return tls.Certificate{}, fmt.Errorf("reading the agent identity %s; delete it to create a new one and adopt the agent again: %w", path, err)
	}

	return identity, nil
}

func loadPairing(dataPath string) (pairing, error) {
	var paired pairing

	data, err := os.ReadFile(filepath.Join(dataPath, pairingFile))

	if errors.Is(err, os.ErrNotExist) {
		return paired, nil
	}

	if err != nil {
		return paired, fmt.Errorf("reading the pairing: %w", err)
	}

	if err := json.Unmarshal(data, &paired); err != nil {
		return paired, fmt.Errorf("reading the pairing %s; delete it to let DockVMap adopt the agent again: %w", filepath.Join(dataPath, pairingFile), err)
	}

	return paired, nil
}

func savePairing(dataPath string, paired pairing) error {
	data, err := json.Marshal(paired)

	if err != nil {
		return err
	}

	return writeFileAtomic(filepath.Join(dataPath, pairingFile), data, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")

	if err != nil {
		return err
	}

	tmpName := tmp.Name()

	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}
