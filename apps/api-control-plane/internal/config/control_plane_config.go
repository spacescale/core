package config

import "time"

const (
	defaultControlPlaneAddr              = ":9090"
	defaultControlPlaneHeartbeatInterval = 10 * time.Second
	defaultControlPlaneLeaseTTL          = 45 * time.Second
	defaultControlPlaneLastSeenFlush     = 30 * time.Second
	defaultControlPlaneMaxRecvMsgBytes   = 1 << 20
	defaultControlPlaneMaxSendMsgBytes   = 1 << 20
)

// ControlPlaneConfig defines runtime settings for the control-plane server.
type ControlPlaneConfig struct {
	Addr              string
	AgentToken        string
	HeartbeatInterval time.Duration
	LeaseTTL          time.Duration
	LastSeenFlush     time.Duration
	MaxRecvMsgBytes   int
	MaxSendMsgBytes   int
}

// Normalized applies safe runtime defaults and invariants.
func (c ControlPlaneConfig) Normalized() ControlPlaneConfig {
	if c.Addr == "" {
		c.Addr = defaultControlPlaneAddr
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = defaultControlPlaneHeartbeatInterval
	}
	if c.LeaseTTL < c.HeartbeatInterval*2 {
		c.LeaseTTL = c.HeartbeatInterval * 3
	}
	if c.LastSeenFlush <= 0 {
		c.LastSeenFlush = defaultControlPlaneLastSeenFlush
	}
	if c.MaxRecvMsgBytes <= 0 {
		c.MaxRecvMsgBytes = defaultControlPlaneMaxRecvMsgBytes
	}
	if c.MaxSendMsgBytes <= 0 {
		c.MaxSendMsgBytes = defaultControlPlaneMaxSendMsgBytes
	}
	return c
}
