package replicationapi

import (
	"errors"
	"testing"
)

func TestTokenRoundTrip(t *testing.T) {
	fingerprint := "sha256:" + hex64

	token, secret, err := NewToken(fingerprint)

	if err != nil {
		t.Fatalf("NewToken() error = %v", err)
	}

	gotFingerprint, gotSecret, err := ParseToken(token)

	if err != nil || gotFingerprint != fingerprint || gotSecret != secret {
		t.Errorf("ParseToken() = %q, %q, %v, want %q, %q", gotFingerprint, gotSecret, err, fingerprint, secret)
	}
}

func TestParseTokenRejectsMalformedTokens(t *testing.T) {
	for _, token := range []string{"", "dvmr1-abc-def", "dvmr2-" + hex64 + "-" + hex64, "dvmr1-" + hex64 + "-" + hex64 + "-x", "dvmr1-" + hex64 + "-" + hex64[:63] + "Z"} {
		if _, _, err := ParseToken(token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("ParseToken(%q) error = %v, want ErrInvalidToken", token, err)
		}
	}
}

const hex64 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestMismatchesComparesOnlySettingsBothSidesKnow(t *testing.T) {
	follower := Setup{"virtual_tag": "current", "proxy_auth.enabled": "false", "REGISTRY_STORAGE_DELETE_ENABLED": "true"}
	primary := Setup{"virtual_tag": "current", "proxy_auth.enabled": "true", "added_in_a_newer_primary": "x"}

	mismatches := follower.Mismatches(primary)

	if len(mismatches) != 1 || mismatches[0] != (Mismatch{Setting: "proxy_auth.enabled", Primary: "true", Follower: "false"}) {
		t.Errorf("Mismatches() = %+v, want only proxy_auth.enabled", mismatches)
	}
}
