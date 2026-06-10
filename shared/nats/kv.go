// Package nats provides shared transport and storage primitives for SpaceScale.
//
// This file specifically manages NATS JetStream Key-Value (KV) stores. In the
// SpaceScale architecture, the KV store is NOT used for placement data or
// permanent state which lives in Postgres.
//
// Instead, the KV store acts as the cluster's "Immune System" and ephemeral
// liveness registry. Daemons write their heartbeat to an in-memory bucket with
// a strict Time-To-Live (TTL). If a node dies, NATS automatically deletes the key,
// triggering the Control Plane's Watcher to re-auction the orphaned workloads.
package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const (
	// NodeHeartbeatBucket is the name of the JetStream KV bucket for liveness.
	NodeHeartbeatBucket = "NODE_HEARTBEATS"

	// NodeHeartbeatKeyPrefix namespaces the keys within the bucket.
	NodeHeartbeatKeyPrefix = "nodes"

	// NodeHeartbeatTTL is the dead-man's switch duration. If a node fails to
	// pulse within this window, NATS automatically purges its key.
	NodeHeartbeatTTL = 15 * time.Second
)

// KeyValue is a NATS JetStream key-value bucket handle.
type KeyValue = jetstream.KeyValue

// JetStream upgrades the standard NATS connection to a JetStream context.
// It assumes the underlying Client and connection are successfully initialized.
func (c *Client) JetStream() (jetstream.JetStream, error) {
	return jetstream.New(c.conn)
}

// EnsureKeyValue creates a new JetStream KV bucket or updates it if it already
// exists with differing configuration parameters.
func (c *Client) EnsureKeyValue(ctx context.Context, cfg jetstream.KeyValueConfig) (KeyValue, error) {
	js, err := c.JetStream()
	if err != nil {
		return nil, err
	}
	// CreateOrUpdateKeyValue handles idempotency automatically.
	return js.CreateOrUpdateKeyValue(ctx, cfg)
}

// EnsureNodeHeartbeatKV provisions the ephemeral liveness bucket.
// It forces MemoryStorage because this data is highly transient and rebuilding
// it after a NATS server restart requires no durable state.
func (c *Client) EnsureNodeHeartbeatKV(ctx context.Context) (KeyValue, error) {
	var cfg jetstream.KeyValueConfig
	cfg.Bucket = NodeHeartbeatBucket
	cfg.Description = "Ephemeral liveness state of all scaled daemon nodes"
	cfg.TTL = NodeHeartbeatTTL
	cfg.History = 1 // We only care about the absolute latest pulse.
	cfg.Storage = jetstream.MemoryStorage

	return c.EnsureKeyValue(ctx, cfg)
}

// NodeHeartbeatKey generates the specific KV key for a given node.
// Format: nodes.<nodeID>
func NodeHeartbeatKey(nodeID string) string {
	return NodeHeartbeatKeyPrefix + "." + nodeID
}

// NodeHeartbeatWatchAll returns the NATS wildcard routing key used by the
// Control Plane's reconciliation loop to watch all node lifecycles.
func NodeHeartbeatWatchAll() string {
	return NodeHeartbeatKeyPrefix + ".>"
}

// PutProtoKV marshals a Protobuf message and writes it directly to the KV bucket.
func PutProtoKV(ctx context.Context, keyValue KeyValue, key string, msg proto.Message) (uint64, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal proto for key %q: %w", key, err)
	}

	return keyValue.Put(ctx, key, payload)
}
