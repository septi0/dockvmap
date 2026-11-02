package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const secretTokenBytes = 32

func generateSecretToken() (string, error) {
	buf := make([]byte, secretTokenBytes)

	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func hashSecretToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
