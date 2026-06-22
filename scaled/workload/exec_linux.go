//go:build linux

// Package workload handles targeted microVM execution commands after placement wins.
//
// The package validates execution messages, commits reserved capacity, invokes the
// local microvm vmm, and publishes accepted only after guestd hello. On boot
// or reply failure it reverts capacity and tears down any started VM.
package workload

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/spacescale/core/scaled/workload/microvm"
	"github.com/spacescale/core/shared/nats"
	"github.com/spacescale/core/shared/pb/v1"
)

const (
	// execBootTimeout is a failure guard, not the expected boot time.
	// The guestd path should be fast; if Firecracker cannot start and send hello
	// inside this window, the node should reject the execution and free capacity.
	execBootTimeout = 10 * time.Second
)

// vmm abstracts the microVM lifecycle boundary used by the executor.
// The concrete microvm.Launcher owns Linux, Firecracker, and KVM-specific
// boot behavior that sits below workload orchestration.
type vmm interface {
	Launch(ctx context.Context, request microvm.LaunchRequest) (*microvm.ActiveVM, error)
	Stop(ctx context.Context, microvmID string) error
}

// imageConfigResolver names the host-side OCI lookup dependency used by the executor.
//
// Keeping it as a field on executor lets tests replace the registry lookup
// with a small stub without pulling registry behavior into every execution test.
type imageConfigResolver func(context.Context, string) (resolvedOCIConfig, error)

// artifactMaterializer names the host-side workload image build dependency.
type artifactMaterializer func(context.Context, artifactCacheScope, string) (materializedArtifact, error)

// executor handles targeted microVM execution commands after placement wins.
type executor struct {
	logger              *slog.Logger
	capacity            *Capacity
	bootID              string
	vmm                 vmm
	resolveImageConfig  imageConfigResolver
	materializeWorkload artifactMaterializer
}

// newExecutor constructs an executor for one node boot identity.
func newExecutor(logger *slog.Logger, capacity *Capacity, bootID string, vmm vmm) *executor {
	return &executor{
		logger:              logger,
		capacity:            capacity,
		bootID:              bootID,
		vmm:                 vmm,
		resolveImageConfig:  resolveOCIConfig,
		materializeWorkload: materializeArtifact,
	}
}

// register connects the executor to the node's specific targeted inbox.
// This subject includes the boot ID to guarantee that stale execution commands
// from previous boot lifecycles are naturally dropped.
func (e *executor) register(ctx context.Context, client *nats.Client) (string, error) {
	subject := nats.NodeMicroVMLaunchSubject(e.bootID)
	_, err := client.Subscribe(subject, func(msg *nats.Msg) error {
		return e.handle(ctx, client, msg)
	})
	if err != nil {
		return "", err
	}
	return subject, nil
}

// handle validates a targeted execution command, resolves OCI runtime metadata,
// boots the local microVM, and only then acknowledges the request.
//
// The order matters: once capacity is committed, any failure in OCI lookup,
// request resolution, VM boot, or reply publication must revert that
// capacity before returning.
func (e *executor) handle(ctx context.Context, client *nats.Client, msg *nats.Msg) error {
	if msg.Reply == "" {
		return errors.New("microvm launch request missing reply subject")
	}

	var req pb.MicroVMLaunchRequest
	if err := nats.UnmarshalProto(msg, &req); err != nil {
		return err
	}
	if req.GetMicrovmId() == "" {
		return errors.New("microvm launch request missing microvm id")
	}

	if _, err := SpecFromShape(req.GetShape()); err != nil {
		return err
	}

	committedSpec, ok := e.capacity.Commit(req.GetMicrovmId())
	if !ok {
		return client.PublishProto(msg.Reply, &pb.MicroVMLaunchResponse{
			Accepted:     false,
			ErrorMessage: "reservation expired or not found",
		})
	}

	e.logger.Info("won microvm placement auction",
		"microvm_id", req.GetMicrovmId(),
		"vcpu", committedSpec.VCPU,
		"ram_mb", committedSpec.RAM,
		"cpu_mode", CPUModeLogValue(req.GetShape()),
		"volume_mb", req.GetShape().GetVolumeMb(),
	)

	execCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), execBootTimeout)
	defer cancel()

	cfg, err := e.resolveImageConfig(execCtx, req.GetImageRef())
	if err != nil {
		return e.rejectExecution(client, msg.Reply, committedSpec, err)
	}

	artifact, err := e.materializeWorkload(execCtx, artifactCacheScope{
		WorkspaceID:      req.GetWorkspaceId(),
		SourceVisibility: imageSourcePublic,
	}, req.GetImageRef())
	if err != nil {
		return e.rejectExecution(client, msg.Reply, committedSpec, err)
	}

	launchReq, err := resolveLaunchRequest(
		req.GetMicrovmId(),
		committedSpec,
		req.GetImageRef(),
		req.GetEnv(),
		req.GetRuntimePort(),
		artifact.ArtifactPath,
		cfg,
	)
	if err != nil {
		return e.rejectExecution(client, msg.Reply, committedSpec, err)
	}

	active, err := e.vmm.Launch(execCtx, launchReq)
	if err != nil {
		return e.rejectExecution(client, msg.Reply, committedSpec, err)
	}

	reply := &pb.MicroVMLaunchResponse{
		Accepted: true,
	}

	if err := client.PublishProto(msg.Reply, reply); err != nil {
		if active != nil {
			err = errors.Join(err, e.vmm.Stop(context.WithoutCancel(ctx), active.MicroVMID))
		}
		e.capacity.Revert(committedSpec)
		return err
	}

	return nil
}

func (e *executor) rejectExecution(client *nats.Client, replySubject string, committedSpec HardwareSpec, err error) error {
	e.capacity.Revert(committedSpec)
	publishErr := client.PublishProto(replySubject, &pb.MicroVMLaunchResponse{
		Accepted:     false,
		ErrorMessage: err.Error(),
	})

	return errors.Join(err, publishErr)
}
