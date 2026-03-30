package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spacescale/core/internal/shared/nats"
	scalepb "github.com/spacescale/core/internal/shared/pb/v1"
)

const bootstrapRequestTimeout = 5 * time.Second

var (
	ErrInvalidBootstrapResponse = errors.New("invalid node bootstrap response")
)

type BootstrapInfo struct {
	Version      string
	BootID       string
	TotalThreads uint32
	TotalRamMb   uint64
	TotalDiskMb  uint64
}

func LoadOrRegisterIdentity(ctx context.Context, client *nats.Client, info BootstrapInfo) (Identity, bool, error) {
	identity, err := LoadIdentity()
	switch {
	case err == nil:
		return identity, false, nil

	case !errors.Is(err, ErrIdentityNotFound):
		return Identity{}, false, err
	}

	token, err := LoadBootstrapToken()
	if err != nil {
		return Identity{}, false, err
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
		return Identity{}, false, fmt.Errorf("bootstrap request failed: %w", err)
	}

	if bootstrapErr := resp.GetError(); bootstrapErr != "" {
		return Identity{}, false, fmt.Errorf("bootstrap rejected: %s", bootstrapErr)
	}

	identity = Identity{
		NodeID: resp.GetNodeId(),
		Region: resp.GetRegion(),
	}
	if identity.NodeID == "" || identity.Region == "" {
		return Identity{}, false, ErrInvalidBootstrapResponse
	}

	if err := SaveIdentity(identity); err != nil {
		return Identity{}, false, err
	}

	if err := DeleteBootstrapToken(); err != nil {
		return Identity{}, false, err
	}

	return identity, true, nil
}
