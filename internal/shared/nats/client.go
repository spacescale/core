package nats

import (
	"fmt"
	"log/slog"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const (
	defaultFlushTimeout = 5 * time.Second
)

// Client wraps a NATS connection with a small API used by Spacescale.
type Client struct {
	name   string
	conn   *natsgo.Conn
	logger *slog.Logger
}

type Msg = natsgo.Msg
type Subscription = natsgo.Subscription

// Handler processes one NATS message.
type Handler func(*Msg) error

// New creates a connected NATS client with Spacescale logging hooks.
func New(url, name string, logger *slog.Logger) (*Client, error) {
	c := &Client{
		name:   name,
		logger: logger,
	}

	nc, err := natsgo.Connect(
		url,
		natsgo.Name("spacescale-"+name),
		natsgo.MaxReconnects(-1),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, err error) {
			if err != nil {
				c.logger.Warn("nats disconnected", "component", "nats", "client", c.name, "error", err)
				return
			}
			c.logger.Warn("nats disconnected", "component", "nats", "client", c.name)
		}),
		natsgo.ReconnectHandler(func(nc *natsgo.Conn) {
			c.logger.Info("nats reconnected", "component", "nats", "client", c.name, "url", nc.ConnectedUrl())
		}),
		natsgo.ClosedHandler(func(nc *natsgo.Conn) {
			if err := nc.LastError(); err != nil {
				c.logger.Warn("nats closed", "component", "nats", "client", c.name, "error", err)
				return
			}
			c.logger.Info("nats closed", "component", "nats", "client", c.name)
		}),
		natsgo.ErrorHandler(func(_ *natsgo.Conn, sub *Subscription, err error) {
			if err == nil {
				return
			}
			subject := ""
			if sub != nil {
				subject = sub.Subject
			}
			c.logger.Warn("nats async error", "component", "nats", "client", c.name, "subject", subject, "error", err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	c.conn = nc
	c.logger.Info("nats connected", "component", "nats", "client", c.name, "url", nc.ConnectedUrl())
	return c, nil
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *natsgo.Conn {
	return c.conn
}

// Publish sends raw bytes to a subject.
func (c *Client) Publish(subject string, payload []byte) error {
	if err := c.conn.Publish(subject, payload); err != nil {
		return fmt.Errorf("publish %q: %w", subject, err)
	}
	return nil
}

// PublishProto marshals and publishes a protobuf message.
func (c *Client) PublishProto(subject string, message proto.Message) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal proto for %q: %w", subject, err)
	}
	return c.Publish(subject, payload)
}

// Subscribe registers a plain subscription handler for a subject.
func (c *Client) Subscribe(subject string, handler Handler) (*Subscription, error) {
	sub, err := c.conn.Subscribe(subject, func(msg *natsgo.Msg) {
		if err := handler(msg); err != nil {
			c.logger.Warn("nats handler failed", "component", "nats", "client", c.name, "subject", msg.Subject, "error", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", subject, err)
	}

	if err := c.Flush(defaultFlushTimeout); err != nil {
		_ = sub.Unsubscribe()
		return nil, err
	}
	return sub, nil
}

// QueueSubscribe registers a queue subscription handler for a subject.
func (c *Client) QueueSubscribe(subject, queue string, handler Handler) (*Subscription, error) {
	sub, err := c.conn.QueueSubscribe(subject, queue, func(msg *Msg) {
		if err := handler(msg); err != nil {
			c.logger.Warn("nats handler failed", "component", "nats", "client", c.name, "subject", msg.Subject, "queue", queue, "error", err)
		}
	})

	if err != nil {
		return nil, fmt.Errorf("queue subscribe %q: %w", subject, err)
	}

	if err := c.Flush(defaultFlushTimeout); err != nil {
		_ = sub.Unsubscribe()
		return nil, err
	}
	return sub, nil
}

// Flush waits for the server to process pending client operations.
func (c *Client) Flush(timeout time.Duration) error {
	if err := c.conn.FlushTimeout(timeout); err != nil {
		return fmt.Errorf("flush nats connection: %w", err)
	}
	return nil
}

// Drain gracefully drains subscriptions and publishers before closing.
func (c *Client) Drain() error {
	if err := c.conn.Drain(); err != nil {
		return fmt.Errorf("drain nats connection: %w", err)
	}
	return nil
}

// Close immediately closes the underlying NATS connection.
func (c *Client) Close() {
	c.conn.Close()
}

// UnmarshalProto decodes a protobuf payload from a NATS message.
func UnmarshalProto(msg *Msg, dst proto.Message) error {
	if err := proto.Unmarshal(msg.Data, dst); err != nil {
		return fmt.Errorf("unmarshal proto from %q: %w", msg.Subject, err)
	}
	return nil
}

// RequestProto marshals a protobuf request and unmarshals the protobuf reply.
func (c *Client) RequestProto(subject string, req, resp proto.Message, timeout time.Duration) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal proto for %q: %w", subject, err)
	}

	msg, err := c.conn.Request(subject, payload, timeout)
	if err != nil {
		return fmt.Errorf("request %q: %w", subject, err)
	}

	if err := UnmarshalProto(msg, resp); err != nil {
		return fmt.Errorf("decode reply for %q: %w", subject, err)
	}

	return nil
}

// Gather broadcasts a request with a private reply inbox and collects all
// replies received during the timeout window.
// It flushes the connection prior to publishing to prevent subscription race conditions.
func (c *Client) Gather(subject string, payload []byte) ([]*Msg, error) {
	inbox := natsgo.NewInbox()              // Create a temporary, unique burner inbox for this specific gather operation.
	sub, err := c.conn.SubscribeSync(inbox) // Synchronously subscribe to the burner inbox. No background goroutines.
	if err != nil {
		return nil, fmt.Errorf("subscribe inbox %q: %w", inbox, err)
	}
	defer sub.Unsubscribe()
	if err := c.Flush(defaultFlushTimeout); err != nil {
		return nil, fmt.Errorf("flush inbox %q: %w", inbox, err)
	}

	if err := c.conn.PublishRequest(subject, inbox, payload); err != nil {
		return nil, fmt.Errorf("publish request %q: %w", subject, err)
	}

	replies := make([]*Msg, 0, 4) // prepare empty slice to hold incoming bid
	deadline := time.Now().Add(defaultGatherTimeout)

	// we use loop to gather the replies
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return replies, nil
		}
		// Block and wait for the next message, but ONLY for the remaining time.
		msg, err := sub.NextMsg(remaining)
		if err != nil {
			if errors.Is(err, natsgo.ErrTimeout) {
				return replies, nil
			}
			return nil, fmt.Errorf("collect replies for %q: %w", subject, err)
		}
		// We caught a bid! Add it to the pile and loop back.
		replies = append(replies, msg)
	}
}

// GatherProto marshals a Protobuf request, executes a Gather, and returns the
// raw slice of reply messages for the caller to unmarshal.
func (c *Client) GatherProto(subject string, req proto.Message) ([]*Msg, error) {
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal gather proto for %q: %w", subject, err)
	}

	return c.Gather(subject, payload)
}
