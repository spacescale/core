package nats

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const (
	defaultFlushTimeout   = 5 * time.Second
	defaultRequestTimeout = 5 * time.Second
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
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("nats url is required")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("nats client name is required")
	}

	if logger == nil {
		logger = slog.Default()
	}

	c := &Client{
		name:   name,
		logger: logger,
	}
	disconnectHandler := func(_ *natsgo.Conn, err error) {
		if err != nil {
			c.logger.Warn("nats disconnected", "component", "nats", "client", c.name, "error", err)
			return
		}

		c.logger.Warn("nats disconnected", "component", "nats", "client", c.name)
	}
	reconnectHandler := func(nc *natsgo.Conn) {
		c.logger.Info("nats reconnected", "component", "nats", "client", c.name, "url", nc.ConnectedUrl())
	}
	closedHandler := func(nc *natsgo.Conn) {
		if err := nc.LastError(); err != nil {
			c.logger.Warn("nats closed", "component", "nats", "client", c.name, "error", err)
			return
		}

		c.logger.Info("nats closed", "component", "nats", "client", c.name)
	}
	errorHandler := func(_ *natsgo.Conn, sub *Subscription, err error) {
		if err == nil {
			return
		}

		subject := ""
		if sub != nil {
			subject = sub.Subject
		}

		c.logger.Warn("nats async error", "component", "nats", "client", c.name, "subject", subject, "error", err)
	}

	nc, err := natsgo.Connect(
		url,
		natsgo.Name("spacescale-"+name),
		natsgo.MaxReconnects(-1),
		natsgo.DisconnectErrHandler(disconnectHandler),
		natsgo.ReconnectHandler(reconnectHandler),
		natsgo.ClosedHandler(closedHandler),
		natsgo.ErrorHandler(errorHandler),
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
	if c == nil {
		return nil
	}
	return c.conn
}

// Publish sends raw bytes to a subject.
func (c *Client) Publish(subject string, payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("nats client is not initialized")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return errors.New("nats subject is required")
	}
	if err := c.conn.Publish(subject, payload); err != nil {
		return fmt.Errorf("publish %q: %w", subject, err)
	}
	return nil
}

// PublishProto marshals and publishes a protobuf message.
func (c *Client) PublishProto(subject string, message proto.Message) error {
	if message == nil {
		return errors.New("proto message is required")
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal proto for %q: %w", subject, err)
	}
	return c.Publish(subject, payload)
}

// Subscribe registers a plain subscription handler for a subject.
func (c *Client) Subscribe(subject string, handler Handler) (*Subscription, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("nats client is not initialized")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, errors.New("nats subject is required")
	}
	if handler == nil {
		return nil, errors.New("nats handler is required")
	}

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
	if c == nil || c.conn == nil {
		return nil, errors.New("nats client is not initialized")
	}

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, errors.New("nats subject is required")
	}

	queue = strings.TrimSpace(queue)
	if queue == "" {
		return nil, errors.New("nats queue is required")
	}

	if handler == nil {
		return nil, errors.New("nats handler is required")
	}
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
	if c == nil || c.conn == nil {
		return errors.New("nats client is not initialized")
	}

	if timeout <= 0 {
		timeout = defaultFlushTimeout
	}

	if err := c.conn.FlushTimeout(timeout); err != nil {
		return fmt.Errorf("flush nats connection: %w", err)
	}
	return nil
}

// Drain gracefully drains subscriptions and publishers before closing.
func (c *Client) Drain() error {
	if c == nil || c.conn == nil {
		return nil
	}
	if err := c.conn.Drain(); err != nil {
		return fmt.Errorf("drain nats connection: %w", err)
	}
	return nil
}

// Close immediately closes the underlying NATS connection.
func (c *Client) Close() {
	if c == nil || c.conn == nil {
		return
	}
	c.conn.Close()
}

// UnmarshalProto decodes a protobuf payload from a NATS message.
func UnmarshalProto(msg *Msg, dst proto.Message) error {
	if msg == nil {
		return errors.New("nats message is required")
	}
	if dst == nil {
		return errors.New("proto destination is required")
	}

	if err := proto.Unmarshal(msg.Data, dst); err != nil {
		return fmt.Errorf("unmarshal proto from %q: %w", msg.Subject, err)
	}
	return nil
}

// RequestProto marshals a protobuf request and unmarshals the protobuf reply.
func (c *Client) RequestProto(subject string, req, resp proto.Message, timeout time.Duration) error {
	if c == nil || c.conn == nil {
		return errors.New("nats client is not initialized")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return errors.New("nats subject is required")
	}
	if req == nil {
		return errors.New("proto request is required")
	}

	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	payload, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal proto for %q: %w", subject, err)
	}

	msg, err := c.conn.Request(subject, payload, timeout)
	if err != nil {
		return fmt.Errorf("request %q: %w", subject, err)
	}

	if resp == nil {
		return errors.New("proto response is required")
	}
	
	if err := UnmarshalProto(msg, resp); err != nil {
		return fmt.Errorf("decode reply for %q: %w", subject, err)
	}

	return nil
}
