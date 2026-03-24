package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spacescale/core/internal/shared/nats"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

const bootstrapRequestTimeout = 5 * time.Second

var (
	ErrInvalidBootstrapInfo     = errors.New("invalid node bootstrap info")
	ErrInvalidBootstrapResponse = errors.New("invalid node bootstrap response")
)

type BootstrapInfo struct {
	Version      string
	BootID       string
	TotalThreads uint32
	TotalRamMb   uint64
	TotalDiskMb  uint64
}

func LoadOrRegisterIdentity(ctx context.Context, client *nats.Client, info BootstrapInfo) (Identity, error) {
	if client == nil {
		return Identity{}, errors.New("nats client is required")
	}

	info, err := normalizeBootstrapInfo(info)
	if err != nil {
		return Identity{}, err
	}

	identity, err := LoadIdentity()
	switch {
	case err == nil:
		return identity, nil

	case !errors.Is(err, ErrIdentityNotFound):
		return Identity{}, err
	}

	token, err := LoadBootstrapToken()
	if err != nil {
		return Identity{}, err
	}

	req := &scalepb.NodeBootstrapRequest{
		BootstrapToken: token,
		Version:        info.Version,
		BootId:         info.BootID,
		TotalThreads:   info.TotalThreads,
		TotalRamMb:     info.TotalRamMb,
		TotalDiskMb:    info.TotalDiskMb,
	}
	resp := &scalepb.NodeBootstrapResponse{}
	if err := client.RequestProto(nats.SubjectNodeBootstrap, req, resp, bootstrapRequestTimeout); err != nil {
		return Identity{}, fmt.Errorf("bootstrap request failed: %w", err)
	}

	if bootstrapErr := strings.TrimSpace(resp.GetError()); bootstrapErr != "" {
		return Identity{}, fmt.Errorf("bootstrap rejected: %s", bootstrapErr)
	}

	identity = Identity{
		NodeID: strings.TrimSpace(resp.GetNodeId()),
		Region: strings.TrimSpace(resp.GetRegion()),
	}
	if identity.NodeID == "" || identity.Region == "" {
		return Identity{}, ErrInvalidBootstrapResponse
	}

	if err := SaveIdentity(identity); err != nil {
		return Identity{}, err
	}

	if err := DeleteBootstrapToken(); err != nil {
		return Identity{}, err
	}

	return identity, nil
}

func normalizeBootstrapInfo(info BootstrapInfo) (BootstrapInfo, error) {
	info.Version = strings.TrimSpace(info.Version)
	info.BootID = strings.TrimSpace(info.BootID)
	if info.Version == "" || info.BootID == "" {
		return BootstrapInfo{}, ErrInvalidBootstrapInfo
	}
	if info.TotalThreads == 0 || info.TotalRamMb == 0 || info.TotalDiskMb == 0 {
		return BootstrapInfo{}, ErrInvalidBootstrapInfo
	}
	return info, nil
}
