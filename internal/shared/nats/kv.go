package nats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const (
	NodeHeartbeatBucket    = "NODE_HEARTBEATS"
	NodeHeartbeatKeyPrefix = "nodes"
	NodeHeartbeatTTL       = 15 * time.Second
)

type JetStream = jetstream.JetStream           // Core interface for stream, consumer, and KV management.
type KeyValue = jetstream.KeyValue             // Dedicated interface for state-based Key-Value operations.
type KeyValueEntry = jetstream.KeyValueEntry   // Represents a single version of a value, including its metadata and revision.
type KeyWatcher = jetstream.KeyWatcher         // A subscription handle used to react to real-time key updates.
type KeyValueConfig = jetstream.KeyValueConfig // The configuration blueprint for bucket behavior (TTL, history, storage).
type KeyValueOp = jetstream.KeyValueOp         // Enum identifying the specific change type (Put, Delete, or Purge).

// JetStream initializes and returns a JetStream management context from the underlying NATS connection.
// It serves as the primary gateway for persistent messaging features, including Streams, Consumers,
// and Key-Value buckets. It returns an error if the Client or the base NATS connection is uninitialized.
func (c *Client) JetStream() (JetStream, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("nats client is not initialized")
	}

	js, err := jetstream.New(c.conn)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}
	return js, nil
}

// EnsureKeyValue idempotently creates or updates a Key-Value (KV) bucket using the provided configuration.
// If the bucket already exists, it returns a handle to the existing bucket after applying any
// configuration updates; otherwise, it creates a new one. This method is essential for
// initializing state-dependent storage like node heartbeat buckets at startup.
func (c *Client) EnsureKeyValue(ctx context.Context, cfg KeyValueConfig) (KeyValue, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	if cfg.Bucket == "" {
		return nil, errors.New("key value bucket is required")
	}

	js, err := c.JetStream()
	if err != nil {
		return nil, err
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create or update key value bucket %q: %w", cfg.Bucket, err)
	}
	return kv, nil
}

// EnsureNodeHeartbeatKV is a specialized, idempotent wrapper that initializes
// the Key-Value bucket specifically for node heartbeats. It enforces a strict
// 15-second TTL (Time-To-Live) and MemoryStorage to ensure that stale heartbeats
// are automatically purged, preventing the Control Plane from scheduling
// workloads on nodes that have silently gone dark.
func (c *Client) EnsureNodeHeartbeatKV(ctx context.Context) (KeyValue, error) {
	return c.EnsureKeyValue(ctx, KeyValueConfig{
		Bucket:  NodeHeartbeatBucket,     // namespace where heartbeats live
		TTL:     NodeHeartbeatTTL,        // 15s: Pulse must be fresh
		History: 1,                       // Only care about the *latest* reality
		Storage: jetstream.MemoryStorage, // High performance, zero disk I/O
	})
}

// NodeHeartbeatKey constructs a standardized, hierarchical NATS Key-Value key for
// a node's heartbeat. It follows the format: "nodes.<region>.<node_id>".
//
// This structure is intentional: it allows the Control Plane to use NATS wildcards
// to watch for updates across the entire fleet ("nodes.>") or restricted to a
// specific region ("nodes.us-east-1.>"). It returns an error if the region or
// nodeID contains invalid characters (dots, spaces, or wildcards) that would
// corrupt the NATS subject routing.
func NodeHeartbeatKey(region, nodeID string) (string, error) {
	region, err := normalizeKeyToken(region, "region")
	if err != nil {
		return "", err
	}

	nodeID, err = normalizeKeyToken(nodeID, "node id")
	if err != nil {
		return "", err
	}

	return NodeHeartbeatKeyPrefix + "." + region + "." + nodeID, nil
}

// NodeHeartbeatWatchAll returns the NATS full-wildcard pattern used to observe
// every heartbeat across the entire global fleet.
//
// It appends the NATS full-wildcard symbol (">") to the heartbeat prefix,
// instructing the Key-Watcher to deliver updates from every region and every
// node (e.g., "nodes.us-east-1.node-a" and "nodes.eu-central-1.node-b"). This
// is the primary pattern used by the central scheduler to maintain a real-time
// inventory of the "Physical Truth" in memory.
func NodeHeartbeatWatchAll() string {
	return NodeHeartbeatKeyPrefix + ".>"
}

// NodeHeartbeatWatchRegion returns the NATS wildcard pattern used to observe
// heartbeats exclusively within a specific geographic or logical region.
//
// It constructs a pattern like "nodes.us-east-1.>" which captures every node
// heartbeat belonging to that specific region prefix. This is the primary
// mechanism for "Regional Sharding," allowing a localized scheduler or
// monitoring agent to ignore global traffic and focus only on the hardware
// it directly manages. It returns an error if the region name is invalid.
func NodeHeartbeatWatchRegion(region string) (string, error) {
	region, err := normalizeKeyToken(region, "region")
	if err != nil {
		return "", err
	}
	return NodeHeartbeatKeyPrefix + "." + region + ".>", nil
}

// PutProtoKV marshals a Protobuf message and stores it in the provided NATS Key-Value bucket.
// It performs validation checks for nil inputs before serializing the message into binary
// format. On a successful write, it returns the NATS revision number (the atomic version
// count for that specific key) which can be used for optimistic concurrency control
// (compare-and-swap operations). Returns an error if marshaling fails or the KV
// write is rejected by the NATS server.
func PutProtoKV(ctx context.Context, kv KeyValue, key string, msg proto.Message) (uint64, error) {
	if ctx == nil {
		return 0, errors.New("context is required")
	}
	if kv == nil {
		return 0, errors.New("key value store is required")
	}

	if msg == nil {
		return 0, errors.New("proto message is required")
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal proto for key %q: %w", key, err)
	}
	revision, err := kv.Put(ctx, key, payload)
	if err != nil {
		return 0, fmt.Errorf("put key %q: %w", key, err)
	}

	return revision, nil
}

func UnmarshalEntryProto(entry KeyValueEntry, dst proto.Message) error {
	if entry == nil {
		return errors.New("key value entry is required")
	}
	if dst == nil {
		return errors.New("proto destination is required")
	}
	value := entry.Value()
	if len(value) == 0 {
		return fmt.Errorf("key value entry %q has no value", entry.Key())
	}
	if err := proto.Unmarshal(value, dst); err != nil {
		return fmt.Errorf("unmarshal proto from key %q: %w", entry.Key(), err)
	}
	return nil
}

func normalizeKeyToken(value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if strings.Contains(value, ".") {
		return "", fmt.Errorf("%s must not contain dot", name)
	}
	if strings.ContainsAny(value, "*> \t\r\n") {
		return "", fmt.Errorf("%s contains invalid characters", name)
	}
	return value, nil
}
