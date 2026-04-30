// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// Package nats
// kv.go provides the shared transport and storage primitives for SpaceScale.
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
	"errors"
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

// Type aliases for easier consumption by downstream SpaceScale packages
// without needing to import the NATS jetstream package directly.

type JetStream = jetstream.JetStream
type KeyValue = jetstream.KeyValue
type KeyValueEntry = jetstream.KeyValueEntry
type KeyWatcher = jetstream.KeyWatcher
type KeyValueConfig = jetstream.KeyValueConfig
type KeyValueOp = jetstream.KeyValueOp

// JetStream upgrades the standard NATS connection to a JetStream context.
// It assumes the underlying Client and connection are successfully initialized.
func (c *Client) JetStream() (JetStream, error) {
	return jetstream.New(c.conn)
}

// EnsureKeyValue creates a new JetStream KV bucket or updates it if it already
// exists with differing configuration parameters.
func (c *Client) EnsureKeyValue(ctx context.Context, cfg KeyValueConfig) (KeyValue, error) {
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
	return c.EnsureKeyValue(ctx, KeyValueConfig{
		Bucket:      NodeHeartbeatBucket,
		Description: "Ephemeral liveness state of all scaled daemon nodes",
		TTL:         NodeHeartbeatTTL,
		History:     1, // We only care about the absolute latest pulse
		Storage:     jetstream.MemoryStorage,
	})
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
func PutProtoKV(ctx context.Context, kv KeyValue, key string, msg proto.Message) (uint64, error) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal proto for key %q: %w", key, err)
	}
	return kv.Put(ctx, key, payload)
}

// GetProtoKV reads a key from the bucket and unmarshals it into the provided
// Protobuf destination. It returns a boolean indicating if the key was found,
// cleanly abstracting the NATS ErrKeyNotFound logic.
func GetProtoKV(ctx context.Context, kv KeyValue, key string, dst proto.Message) (bool, uint64, error) {
	entry, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, 0, nil // Cleanly handle standard absence
		}
		return false, 0, fmt.Errorf("get key %q: %w", key, err)
	}

	if err := UnmarshalEntryProto(entry, dst); err != nil {
		return false, 0, err
	}

	return true, entry.Revision(), nil
}

// UnmarshalEntryProto is a helper to extract and decode the raw bytes from a
// KeyValueEntry into a structured Protobuf message.
func UnmarshalEntryProto(entry KeyValueEntry, dst proto.Message) error {
	if err := proto.Unmarshal(entry.Value(), dst); err != nil {
		return fmt.Errorf("unmarshal proto from key %q: %w", entry.Key(), err)
	}
	return nil
}
