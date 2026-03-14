package config

import (
	"regexp"
	"testing"
	"time"
)

func setBaselineEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CONTROL_PLANE_AGENT_TOKEN", "token-1")
	t.Setenv("SCALED_AGENT_KEY", "agent-key-1")
	t.Setenv("SCALED_AGENT_NAME", "node-test")
	t.Setenv("SCALED_AGENT_VERSION", "v-test")
}

func TestLoadFromEnvMissingAgentToken(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("CONTROL_PLANE_AGENT_TOKEN", "")

	_, err := LoadFromEnv()
	if err == nil || err.Error() != "missing required config CONTROL_PLANE_AGENT_TOKEN" {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestLoadFromEnvMissingAgentKey(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("SCALED_AGENT_KEY", "")

	_, err := LoadFromEnv()
	if err == nil || err.Error() != "missing required config SCALED_AGENT_KEY" {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

func TestLoadFromEnvParsesOverrides(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("SCALED_CONTROL_PLANE_ADDR", "10.0.0.20:9191")
	t.Setenv("SCALED_CALLER", "scaled/bootstrap")
	t.Setenv("SCALED_STATUS", "draining")
	t.Setenv("SCALED_CAPABILITIES", "firecracker, logs, heartbeat,logs")
	t.Setenv("SCALED_GRPC_DIAL_TIMEOUT", "12s")
	t.Setenv("SCALED_GRPC_HEALTH_TIMEOUT", "7s")
	t.Setenv("SCALED_GRPC_MIN_CONNECT_TIMEOUT", "6s")
	t.Setenv("SCALED_RECONNECT_INITIAL_BACKOFF", "2s")
	t.Setenv("SCALED_RECONNECT_MAX_BACKOFF", "20s")
	t.Setenv("SCALED_HEARTBEAT_FALLBACK_INTERVAL", "15s")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.ControlPlane.Addr != "10.0.0.20:9191" {
		t.Fatalf("unexpected control-plane addr: %s", cfg.ControlPlane.Addr)
	}
	if cfg.Agent.Caller != "scaled/bootstrap" {
		t.Fatalf("unexpected caller: %s", cfg.Agent.Caller)
	}
	if cfg.Agent.Status != "draining" {
		t.Fatalf("unexpected status: %s", cfg.Agent.Status)
	}
	if len(cfg.Agent.Capabilities) != 3 {
		t.Fatalf("expected deduplicated capabilities, got %#v", cfg.Agent.Capabilities)
	}
	if cfg.ControlPlane.DialTimeout != 12*time.Second {
		t.Fatalf("unexpected dial timeout: %s", cfg.ControlPlane.DialTimeout)
	}
	if cfg.ControlPlane.HealthTimeout != 7*time.Second {
		t.Fatalf("unexpected health timeout: %s", cfg.ControlPlane.HealthTimeout)
	}
	if cfg.ControlPlane.MinConnectTimeout != 6*time.Second {
		t.Fatalf("unexpected min connect timeout: %s", cfg.ControlPlane.MinConnectTimeout)
	}
	if cfg.ControlPlane.ReconnectInitialBackoff != 2*time.Second {
		t.Fatalf("unexpected reconnect initial backoff: %s", cfg.ControlPlane.ReconnectInitialBackoff)
	}
	if cfg.ControlPlane.ReconnectMaxBackoff != 20*time.Second {
		t.Fatalf("unexpected reconnect max backoff: %s", cfg.ControlPlane.ReconnectMaxBackoff)
	}
	if cfg.ControlPlane.HeartbeatFallbackInterval != 15*time.Second {
		t.Fatalf("unexpected heartbeat fallback interval: %s", cfg.ControlPlane.HeartbeatFallbackInterval)
	}
}

func TestLoadFromEnvComputesNameAndVersionFallback(t *testing.T) {
	setBaselineEnv(t)
	t.Setenv("SCALED_AGENT_NAME", "")
	t.Setenv("SCALED_AGENT_VERSION", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Agent.Name == "" {
		t.Fatal("expected computed agent name")
	}
	matched, err := regexp.MatchString(`^[a-z0-9-]+-[0-9a-f]{6}$`, cfg.Agent.Name)
	if err != nil {
		t.Fatalf("regex compile failed: %v", err)
	}
	if !matched {
		t.Fatalf("unexpected computed agent name format: %s", cfg.Agent.Name)
	}
	if cfg.Agent.Version == "" {
		t.Fatal("expected computed agent version")
	}
}
