package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"
	"unicode"
)

// These constants define the fallback values used if the environment variables are missing or invalid.
// Using sensible defaults ensures the agent can at least attempt to start up even with minimal config.
const (
	defaultControlPlaneAddr          = "127.0.0.1:9090" // Standard local address for the API
	defaultDialTimeout               = 10 * time.Second // How long to wait for the initial TCP connection
	defaultHealthTimeout             = 5 * time.Second  // Short timeout for the fast "Is the API alive?" check
	defaultMinConnectTimeout         = 5 * time.Second  // Minimum time to wait before declaring a connection attempt failed
	defaultReconnectInitialBackoff   = 1 * time.Second  // First wait time after a disconnection
	defaultReconnectMaxBackoff       = 30 * time.Second // Longest wait time (prevents waiting forever if server is down)
	defaultHeartbeatFallbackInterval = 10 * time.Second // Used if the server doesn't specify an interval
	defaultAgentCaller               = "scaled/startup" // Internal label identifying this code as the request source
	defaultAgentStatus               = "ready"           // Initial status of the agent
)

// defaultAgentCapabilities represents the standard set of skills this agent version can provide.
var defaultAgentCapabilities = []string{"firecracker", "logs", "heartbeat"}

// Config is the "Master Container" for every setting the agent needs to run.
// By nesting configs, we keep a clean separation between "Who am I?" (Agent)
// and "Where am I going?" (ControlPlane).
type Config struct {
	Agent        AgentConfig
	ControlPlane ControlPlaneConfig
}

// AgentConfig manages the identity of this machine. 
// In a distributed system, identity is everything—it's how the Control Plane 
// tracks heartbeats and assigns specific workloads to specific nodes.
type AgentConfig struct {
	Key          string   // The "Secret ID" (Password) shared with the Control Plane
	Name         string   // The human-readable name shown in dashboards (e.g., "node-01-abc123")
	Version      string   // The specific build version or Git commit of this agent
	Caller       string   // A label that tags every request for easier debugging in logs
	Status       string   // The current lifecycle state (ready, draining, etc.)
	Capabilities []string // The list of "Skills" this agent can perform (e.g., "firecracker")
	StartedUnix  int64    // The precise timestamp (Unix) when this process began
}

// ControlPlaneConfig manages the "Conversation" settings between the Agent and the API.
// It controls how we connect, how we handle errors, and how we "Backoff" when retrying.
type ControlPlaneConfig struct {
	Addr                      string        // The network address of the gRPC Control Plane
	AgentToken                string        // The Bearer token used for authentication
	DialTimeout               time.Duration // Max time to wait for the network "Dial" to finish
	HealthTimeout             time.Duration // Max time for the pre-registration health check
	MinConnectTimeout         time.Duration // Minimum window for a connection attempt
	ReconnectInitialBackoff   time.Duration // The "Starting timer" for our exponential retry logic
	ReconnectMaxBackoff       time.Duration // The "Cap" on our retry timer to avoid excessive delays
	HeartbeatFallbackInterval time.Duration // The backup interval if the server doesn't tell us how often to pulse
}

// LoadFromEnv is the entry point for configuration. It "Harvests" variables from the OS 
// environment and turns them into typed Go objects. It also performs "Self-Healing" 
// by computing missing identities (like Name and Version) automatically.
func LoadFromEnv() (Config, error) {
	// 1. Load the Control Plane connection settings first.
	controlPlane := ControlPlaneConfig{
		Addr:                      envStr("SCALED_CONTROL_PLANE_ADDR", defaultControlPlaneAddr),
		AgentToken:                envStr("CONTROL_PLANE_AGENT_TOKEN", ""),
		DialTimeout:               parseEnvDuration("SCALED_GRPC_DIAL_TIMEOUT", defaultDialTimeout),
		HealthTimeout:             parseEnvDuration("SCALED_GRPC_HEALTH_TIMEOUT", defaultHealthTimeout),
		MinConnectTimeout:         parseEnvDuration("SCALED_GRPC_MIN_CONNECT_TIMEOUT", defaultMinConnectTimeout),
		ReconnectInitialBackoff:   parseEnvDuration("SCALED_RECONNECT_INITIAL_BACKOFF", defaultReconnectInitialBackoff),
		ReconnectMaxBackoff:       parseEnvDuration("SCALED_RECONNECT_MAX_BACKOFF", defaultReconnectMaxBackoff),
		HeartbeatFallbackInterval: parseEnvDuration("SCALED_HEARTBEAT_FALLBACK_INTERVAL", defaultHeartbeatFallbackInterval),
	}.normalized()

	// 2. Security Check: If we don't have a token, we can't talk to the server. Stop now.
	if controlPlane.AgentToken == "" {
		return Config{}, errors.New("missing required config CONTROL_PLANE_AGENT_TOKEN")
	}

	// 3. Identity Check: The Agent Key is the most important "Secret" for a machine.
	agentKey := strings.TrimSpace(envStr("SCALED_AGENT_KEY", ""))
	if agentKey == "" {
		return Config{}, errors.New("missing required config SCALED_AGENT_KEY")
	}

	// 4. Name Generation: If the user didn't name the machine, we compute a unique one.
	agentName := strings.TrimSpace(envStr("SCALED_AGENT_NAME", ""))
	if agentName == "" {
		agentName = computedAgentName(agentKey)
	}

	// 5. Version Extraction: We look inside the binary to see what Git commit is running.
	agentVersion := strings.TrimSpace(envStr("SCALED_AGENT_VERSION", ""))
	if agentVersion == "" {
		agentVersion = computedAgentVersion()
	}

	// 6. Cleanup and defaults for the remaining fields.
	caller := strings.TrimSpace(envStr("SCALED_CALLER", defaultAgentCaller))
	if caller == "" {
		caller = defaultAgentCaller
	}
	status := strings.TrimSpace(envStr("SCALED_STATUS", defaultAgentStatus))
	if status == "" {
		status = defaultAgentStatus
	}
	capabilities := parseCapabilities(envStr("SCALED_CAPABILITIES", "heartbeat"))

	return Config{
		Agent: AgentConfig{
			Key:          agentKey,
			Name:         agentName,
			Version:      agentVersion,
			Caller:       caller,
			Status:       status,
			Capabilities: capabilities,
			StartedUnix:  time.Now().Unix(),
		},
		ControlPlane: controlPlane,
	}, nil
}

// normalized ensures the config is "Sanity Checked." It prevents weird crashes 
// (like a negative timeout) by overriding invalid values with safe defaults.
func (c ControlPlaneConfig) normalized() ControlPlaneConfig {
	c.Addr = strings.TrimSpace(c.Addr)
	c.AgentToken = strings.TrimSpace(c.AgentToken)
	if c.Addr == "" {
		c.Addr = defaultControlPlaneAddr
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.HealthTimeout <= 0 {
		c.HealthTimeout = defaultHealthTimeout
	}
	if c.MinConnectTimeout <= 0 {
		c.MinConnectTimeout = defaultMinConnectTimeout
	}
	if c.ReconnectInitialBackoff <= 0 {
		c.ReconnectInitialBackoff = defaultReconnectInitialBackoff
	}
	if c.ReconnectMaxBackoff <= 0 {
		c.ReconnectMaxBackoff = defaultReconnectMaxBackoff
	}
	// Logic: Initial wait can't be longer than the maximum wait.
	if c.ReconnectInitialBackoff > c.ReconnectMaxBackoff {
		c.ReconnectInitialBackoff = c.ReconnectMaxBackoff
	}
	if c.HeartbeatFallbackInterval <= 0 {
		c.HeartbeatFallbackInterval = defaultHeartbeatFallbackInterval
	}
	return c
}

// computedAgentName creates a "Friendly Identity." It uses the machine's hostname 
// so you know where it is, and a hash of the secret key so you know WHO it is.
func computedAgentName(agentKey string) string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "node"
	}
	// Sanitize ensures the name doesn't contain characters that break URLs or logs.
	base := sanitizeName(strings.ToLower(host))
	if base == "" {
		base = "node"
	}
	// We SHA-256 the secret key to get a random-looking but "Consistent" suffix.
	sum := sha256.Sum256([]byte(strings.TrimSpace(agentKey)))
	suffix := hex.EncodeToString(sum[:])[:6] // Just take the first 6 chars for brevity
	return base + "-" + suffix
}

// computedAgentVersion "Peeks" inside the Go binary to find the Git commit hash.
// This is critical for debugging—it tells you exactly which code is running on the machine.
func computedAgentVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		// Look through build settings for the specific Git revision
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				if len(s.Value) > 12 {
					return s.Value[:12] // Standard 12-char short hash
				}
				return s.Value
			}
		}
	}
	return "unknown"
}

// sanitizeName is a "String Janitor." It removes special characters and ensures 
// the resulting name only contains letters, numbers, and hyphens.
func sanitizeName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	lastHyphen := false
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || r == '.':
			// Prevent double hyphens like "node--01"
			if b.Len() == 0 || lastHyphen {
				continue
			}
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// parseCapabilities converts a comma-separated string from .env into a unique list.
// Example: "firecracker, logs, firecracker" -> ["firecracker", "logs"]
func parseCapabilities(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts)) // Memory-efficient "Set" to prevent duplicates
	for _, p := range parts {
		c := strings.TrimSpace(p)
		if c == "" {
			continue
		}
		if _, exists := seen[c]; exists {
			continue // Skip it if we've already seen this skill
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	// Fallback to defaults if the user provided an empty or invalid list.
	if len(out) == 0 {
		return append([]string(nil), defaultAgentCapabilities...)
	}
	return out
}

// parseEnvDuration handles human-readable time strings like "10s" or "5m".
// It includes a warning logger so the operator knows if they made a typo.
func parseEnvDuration(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn(
			"invalid env duration; using default",
			"event", "startup_config_defaulted",
			"key", key,
			"raw_value", raw,
			"default_value", def.String(),
		)
		return def
	}
	return d
}

// envStr is a simple helper to get an environment string or a fallback.
func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
