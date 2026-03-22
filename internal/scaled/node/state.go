package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultStateDir        = "/var/lib/spacescale"
	identityFileName       = "identity.json"
	bootstrapTokenFileName = "bootstrap_token"
)

var (
	ErrIdentityNotFound      = errors.New("node identity not found")
	ErrInvalidIdentity       = errors.New("invalid node identity")
	ErrBootstrapTokenMissing = errors.New("bootstrap token not found")
	ErrInvalidBootstrapToken = errors.New("invalid bootstrap token")
)

type Identity struct {
	NodeID string `json:"node_id"`
	Region string `json:"region"`
}

func identityPath() string {
	return filepath.Join(defaultStateDir, identityFileName)
}

func bootstrapTokenPath() string {
	return filepath.Join(defaultStateDir, bootstrapTokenFileName)
}

func LoadIdentity() (Identity, error) {
	raw, err := os.ReadFile(identityPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Identity{}, ErrIdentityNotFound
		}
		return Identity{}, fmt.Errorf("read node identity: %w", err)
	}
	var identity Identity
	if err := json.Unmarshal(raw, &identity); err != nil {
		return Identity{}, fmt.Errorf("decode node identity: %w", err)
	}
	identity.NodeID = strings.TrimSpace(identity.NodeID)
	identity.Region = strings.TrimSpace(identity.Region)
	if identity.NodeID == "" || identity.Region == "" {
		return Identity{}, ErrInvalidIdentity
	}
	return identity, nil
}

func SaveIdentity(identity Identity) error {
	identity.NodeID = strings.TrimSpace(identity.NodeID)
	identity.Region = strings.TrimSpace(identity.Region)
	if identity.NodeID == "" || identity.Region == "" {
		return ErrInvalidIdentity
	}
	if err := os.Mkdir(defaultStateDir, 0o755); err != nil {
		return fmt.Errorf("create node state dir: %w", err)
	}

	payload, err := json.Marshal(identity)
	if err != nil {
		return fmt.Errorf("encode node identity: %w", err)
	}
	// write to temp to avoid corrupting existing id
	tmpPath := identityPath() + ".tmp"
	if err := os.WriteFile(tmpPath, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write temp node identity: %w", err)
	}
	// if successful we can replace node identity with temp
	if err := os.Rename(tmpPath, identityPath()); err != nil {
		return fmt.Errorf("replace node identity: %w", err)
	}

	return nil
}

func LoadBootstrapToken() (string, error) {
	raw, err := os.ReadFile(bootstrapTokenPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrBootstrapTokenMissing
		}
		return "", fmt.Errorf("read bootstrap token: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", ErrInvalidBootstrapToken
	}
	return token, nil
}

func DeleteBootstrapToken() error {
	if err := os.Remove(bootstrapTokenPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete bootstrap token: %w", err)
	}
	return nil
}
