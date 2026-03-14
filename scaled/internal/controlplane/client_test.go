package controlplane

import (
	"errors"
	"fmt"
	"testing"
	"time"

	controlpb "github.com/t0gun/spacescale/packages/go/pb/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNextBackoffCapsAtMax(t *testing.T) {
	got := nextBackoff(20*time.Second, 30*time.Second)
	if got != 30*time.Second {
		t.Fatalf("expected max-capped backoff, got %s", got)
	}

	got = nextBackoff(5*time.Second, 30*time.Second)
	if got != 10*time.Second {
		t.Fatalf("expected doubled backoff, got %s", got)
	}
}

func TestIsPermanentErrorProtocolViolation(t *testing.T) {
	err := protocolError("first response must be register")
	if !isPermanentError(err) {
		t.Fatal("expected protocol violation to be permanent")
	}
	if !errors.Is(err, errProtocolViolation) {
		t.Fatal("expected wrapped protocol violation sentinel")
	}
}

func TestIsPermanentErrorFromGRPCCodes(t *testing.T) {
	permanent := []codes.Code{
		codes.Unauthenticated,
		codes.PermissionDenied,
		codes.InvalidArgument,
		codes.FailedPrecondition,
	}
	for _, code := range permanent {
		err := status.Error(code, "boom")
		if !isPermanentError(err) {
			t.Fatalf("expected %s to be permanent", code)
		}
	}

	transient := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
	}
	for _, code := range transient {
		err := status.Error(code, "retry")
		if isPermanentError(err) {
			t.Fatalf("expected %s to be transient", code)
		}
	}
}

func TestGRPCCodeUnwrapsNestedErrors(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", status.Error(codes.PermissionDenied, "denied"))
	if got := grpcCode(wrapped); got != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %s", got)
	}

	if got := grpcCode(errors.New("plain-error")); got != codes.Unknown {
		t.Fatalf("expected unknown grpc code for plain error, got %s", got)
	}
}

func TestDirectiveTarget(t *testing.T) {
	if got := directiveTarget(&controlpb.ControlDirective{Target: &controlpb.ControlDirective_Node{Node: &controlpb.NodeDirective{}}}); got != "node" {
		t.Fatalf("expected node target, got %s", got)
	}
	if got := directiveTarget(&controlpb.ControlDirective{Target: &controlpb.ControlDirective_Agent{Agent: &controlpb.AgentDirective{}}}); got != "agent" {
		t.Fatalf("expected agent target, got %s", got)
	}
	if got := directiveTarget(&controlpb.ControlDirective{Target: &controlpb.ControlDirective_Workload{Workload: &controlpb.WorkloadDirective{}}}); got != "workload" {
		t.Fatalf("expected workload target, got %s", got)
	}
	if got := directiveTarget(&controlpb.ControlDirective{}); got != "unknown" {
		t.Fatalf("expected unknown target, got %s", got)
	}
}
