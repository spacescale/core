//go:build linux

package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spacescale/core/internal/scaled/sysinfo"
	"github.com/spacescale/core/internal/shared/nats"
	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

const bootstrapRequestTimeout = 5 * time.Second

var ErrInvalidBootstrapResponse = errors.New("invalid node bootstrap response")

func Bootstrap(ctx context.Context, client *nats.Client) (sysinfo.Snapshot, Identity, error) {
	snapshot, err := sysinfo.Read()
	if err != nil {
		return sysinfo.Snapshot{}, Identity{}, err
	}

	req := &pb.NodeBootstrapRequest{
		BootId:      snapshot.BootID,
		TotalCores:  snapshot.TotalCores,
		TotalRamMb:  snapshot.TotalRamMb,
		TotalDiskMb: snapshot.TotalDiskMb,
	}

	identity, err := loadOrRegisterIdentity(ctx, client, req)
	if err != nil {
		return sysinfo.Snapshot{}, Identity{}, err
	}

	return snapshot, identity, nil
}

func loadOrRegisterIdentity(ctx context.Context, client *nats.Client, req *pb.NodeBootstrapRequest) (Identity, error) {
	identity, err := loadIdentity()
	switch {
	case err == nil:
		return identity, nil
	case !errors.Is(err, ErrIdentityNotFound):
		return Identity{}, err
	}

	token, err := loadBootstrapToken()
	if err != nil {
		return Identity{}, err
	}

	req.BootstrapToken = token
	resp := &pb.NodeBootstrapResponse{}
	if err := client.RequestProto(nats.SubjectNodeBootstrap, req, resp, bootstrapRequestTimeout); err != nil {
		return Identity{}, fmt.Errorf("bootstrap request failed: %w", err)
	}

	if bootstrapErr := resp.GetError(); bootstrapErr != "" {
		return Identity{}, fmt.Errorf("bootstrap rejected: %s", bootstrapErr)
	}

	identity = Identity{
		NodeID: resp.GetNodeId(),
		Region: resp.GetRegion(),
	}
	if identity.NodeID == "" || identity.Region == "" {
		return Identity{}, ErrInvalidBootstrapResponse
	}

	if err := saveIdentity(identity); err != nil {
		return Identity{}, err
	}

	if err := deleteBootstrapToken(); err != nil {
		return Identity{}, err
	}

	return identity, nil
}
