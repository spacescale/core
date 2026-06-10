package nats

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/spacescale/core/shared/pb/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.Buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.Buffer.String()
}

// startTestNATSServer returns an embedded in-process NATS server.
//
// Port -1 tells the server to pick a random OS port, so tests do not clash
// with each other or with any server already running on the host.
func startTestNATSServer(t *testing.T) string {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
	})
	require.NoError(t, err)
	srv.Start()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	return srv.ClientURL()

}

// newTestClient connects the Spacescale NATS wrapper to the test server.
//
// It uses the same New constructor that production code uses, then registers
// a cleanup closure so the connection is torn down after each test.
func newTestClient(t *testing.T, url string) *Client {
	t.Helper()
	client, err := New(url, "test", newTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func newTestClientWithLogger(t *testing.T, url string, logger *slog.Logger) *Client {
	t.Helper()
	client, err := New(url, "test", logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return client
}

type failingProtoMessage struct{}

type failingProtoReflect struct{}

func (failingProtoMessage) ProtoReflect() protoreflect.Message { return failingProtoReflect{} }

func (failingProtoReflect) Descriptor() protoreflect.MessageDescriptor { return nil }
func (failingProtoReflect) Type() protoreflect.MessageType             { return nil }
func (failingProtoReflect) New() protoreflect.Message                  { return failingProtoReflect{} }
func (failingProtoReflect) Interface() protoreflect.ProtoMessage       { return failingProtoMessage{} }
func (failingProtoReflect) Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool) {
}
func (failingProtoReflect) Has(protoreflect.FieldDescriptor) bool { return false }
func (failingProtoReflect) Clear(protoreflect.FieldDescriptor)    {}
func (failingProtoReflect) Get(protoreflect.FieldDescriptor) protoreflect.Value {
	return protoreflect.Value{}
}
func (failingProtoReflect) Set(protoreflect.FieldDescriptor, protoreflect.Value) {}
func (failingProtoReflect) Mutable(protoreflect.FieldDescriptor) protoreflect.Value {
	return protoreflect.Value{}
}
func (failingProtoReflect) NewField(protoreflect.FieldDescriptor) protoreflect.Value {
	return protoreflect.Value{}
}
func (failingProtoReflect) WhichOneof(protoreflect.OneofDescriptor) protoreflect.FieldDescriptor {
	return nil
}
func (failingProtoReflect) GetUnknown() protoreflect.RawFields { return nil }
func (failingProtoReflect) SetUnknown(protoreflect.RawFields)  {}
func (failingProtoReflect) IsValid() bool                      { return true }
func (failingProtoReflect) ProtoMethods() *protoiface.Methods {
	return &protoiface.Methods{
		Marshal: func(protoiface.MarshalInput) (protoiface.MarshalOutput, error) {
			return protoiface.MarshalOutput{}, errors.New("marshal failed")
		},
	}
}

func TestPublishProtoRoundTrip(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	suscriber, err := rawConn.SubscribeSync("test.publish.proto")
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	want := &pb.MicroVMLaunchResponse{
		Accepted:     true,
		ErrorMessage: "ok",
	}

	err = client.PublishProto("test.publish.proto", want)
	require.NoError(t, err)

	msg, err := suscriber.NextMsg(time.Second)
	require.NoError(t, err)

	var got pb.MicroVMLaunchResponse
	require.NoError(t, UnmarshalProto(msg, &got))
	require.Equal(t, want.GetAccepted(), got.GetAccepted())
	require.Equal(t, want.GetErrorMessage(), got.GetErrorMessage())
}

func TestPublishRoundTrip(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	subscriber, err := rawConn.SubscribeSync("test.publish.raw")
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	want := []byte("hello raw nats")

	err = client.Publish("test.publish.raw", want)
	require.NoError(t, err)

	msg, err := subscriber.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, want, msg.Data)
}

func TestRequestProtoRoundTrip(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = rawConn.Subscribe("test.request.proto", func(msg *natsgo.Msg) {
		var req pb.MicroVMLaunchRequest
		require.NoError(t, UnmarshalProto(msg, &req))
		reply := &pb.MicroVMLaunchResponse{
			Accepted:     true,
			ErrorMessage: "reply-ok",
		}
		payload, err := proto.Marshal(reply)
		require.NoError(t, err)
		require.NoError(t, rawConn.Publish(msg.Reply, payload))
	})
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	req := &pb.MicroVMLaunchRequest{
		MicrovmId: "vm-123",
	}
	var resp pb.MicroVMLaunchResponse
	err = client.RequestProto("test.request.proto", req, &resp, time.Second)
	require.NoError(t, err)
	require.True(t, resp.GetAccepted())
	require.Equal(t, "reply-ok", resp.GetErrorMessage())
}

func TestFirstReplyReturnsFirstResponse(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = rawConn.Subscribe("test.first.reply", func(msg *natsgo.Msg) {
		reply := &pb.MicroVMLaunchResponse{
			Accepted:     true,
			ErrorMessage: "first",
		}
		payload, err := proto.Marshal(reply)
		require.NoError(t, err)
		require.NoError(t, rawConn.Publish(msg.Reply, payload))
	})

	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	msg, err := client.FirstReply("test.first.reply", []byte("ping"))
	require.NoError(t, err)

	var got pb.MicroVMLaunchResponse
	require.NoError(t, UnmarshalProto(msg, &got))
	require.True(t, got.GetAccepted())
	require.Equal(t, "first", got.GetErrorMessage())

}

func TestFirstReplyProtoRoundTrip(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = rawConn.Subscribe("test.first.reply.proto", func(msg *natsgo.Msg) {
		var req pb.MicroVMLaunchRequest
		require.NoError(t, UnmarshalProto(msg, &req))

		reply := &pb.MicroVMLaunchResponse{
			Accepted:     true,
			ErrorMessage: req.GetMicrovmId(),
		}
		payload, err := proto.Marshal(reply)
		require.NoError(t, err)
		require.NoError(t, rawConn.Publish(msg.Reply, payload))
	})
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	msg, err := client.FirstReplyProto("test.first.reply.proto", &pb.MicroVMLaunchRequest{MicrovmId: "vm-123"})
	require.NoError(t, err)

	var got pb.MicroVMLaunchResponse
	require.NoError(t, UnmarshalProto(msg, &got))
	require.True(t, got.GetAccepted())
	require.Equal(t, "vm-123", got.GetErrorMessage())
}

func TestFirstReplyReturnsErrNoReply(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = rawConn.Subscribe("test.first.reply.timeout", func(_ *natsgo.Msg) {})
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	msg, err := client.FirstReply("test.first.reply.timeout", []byte("ping"))
	require.Nil(t, msg)
	require.ErrorIs(t, err, ErrNoReply)
}

func TestNewReturnsErrorForInvalidURL(t *testing.T) {
	_, err := New("://bad-url", "test", newTestLogger())
	require.Error(t, err)
}

func TestNewLogsDisconnect(t *testing.T) {
	buf := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))

	srv, err := server.NewServer(&server.Options{Port: -1, JetStream: true})
	require.NoError(t, err)
	srv.Start()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	_ = newTestClientWithLogger(t, srv.ClientURL(), logger)

	srv.Shutdown()
	srv.WaitForShutdown()
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "nats disconnected")
	}, time.Second, 10*time.Millisecond)
}

func TestNewLogsReconnect(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	port := tcpAddr.Port
	require.NoError(t, ln.Close())

	buf := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	startServer := func() *server.Server {
		srv, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: port, JetStream: true})
		require.NoError(t, err)
		srv.Start()
		return srv
	}

	srv := startServer()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	client := newTestClientWithLogger(t, srv.ClientURL(), logger)

	srv.Shutdown()
	srv.WaitForShutdown()

	srv = startServer()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "nats reconnected")
	}, 5*time.Second, 20*time.Millisecond)
	client.Close()
}

func TestNewLogsAsyncError(t *testing.T) {
	buf := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	url := startTestNATSServer(t)
	client := newTestClientWithLogger(t, url, logger)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	sub, err := client.Subscribe("test.async.error", func(*natsgo.Msg) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, sub.SetPendingLimits(1, 1))

	for range 200 {
		require.NoError(t, rawConn.Publish("test.async.error", []byte("x")))
	}
	require.NoError(t, rawConn.Flush())

	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "nats async error")
	}, 5*time.Second, 20*time.Millisecond)
	client.Close()
}

func TestPublishReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	err := client.Publish("bad subject", []byte("payload"))
	require.Error(t, err)
}

func TestPublishProtoReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	err := client.PublishProto("bad subject", &pb.MicroVMLaunchResponse{Accepted: true})
	require.Error(t, err)
}

func TestPublishProtoReturnsErrorForMarshalFailure(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	err := client.PublishProto("test.publish.marshal", failingProtoMessage{})
	require.Error(t, err)
}

func TestSubscribeReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	_, err := client.Subscribe("bad subject", func(*natsgo.Msg) error { return nil })
	require.Error(t, err)
}

func TestSubscribeHandlerErrorIsIgnored(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = client.Subscribe("test.subscribe.error", func(*natsgo.Msg) error {
		return errors.New("boom")
	})
	require.NoError(t, err)

	require.NoError(t, rawConn.Publish("test.subscribe.error", []byte("ping")))
	require.NoError(t, rawConn.Flush())
}

func TestFlushReturnsErrorOnClosedConnection(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	client.Close()
	err := client.Flush(time.Second)
	require.Error(t, err)
}

func TestDrainReturnsErrorOnClosedConnection(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	client.Close()
	err := client.Drain()
	require.Error(t, err)
}

func TestUnmarshalProtoReturnsError(t *testing.T) {
	msg := &natsgo.Msg{Subject: "test.unmarshal.bad", Data: []byte("not proto")}

	var got pb.MicroVMLaunchResponse
	err := UnmarshalProto(msg, &got)
	require.Error(t, err)
}

func TestRequestProtoReturnsDecodeError(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	_, err = rawConn.Subscribe("test.request.decode", func(msg *natsgo.Msg) {
		require.NoError(t, rawConn.Publish(msg.Reply, []byte("not proto")))
	})
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	err = client.RequestProto(
		"test.request.decode",
		&pb.MicroVMLaunchRequest{MicrovmId: "vm-123"},
		&pb.MicroVMLaunchResponse{},
		time.Second,
	)
	require.Error(t, err)
}

func TestRequestProtoReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	err := client.RequestProto("bad subject", &pb.MicroVMLaunchRequest{}, &pb.MicroVMLaunchResponse{}, time.Second)
	require.Error(t, err)
}

func TestRequestProtoReturnsErrorForNilRequest(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	err := client.RequestProto("test.request.nil", nil, &pb.MicroVMLaunchResponse{}, time.Second)
	require.Error(t, err)
}

func TestFirstReplyReturnsErrorOnClosedConnection(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	client.Close()
	msg, err := client.FirstReply("test.first.reply.closed", []byte("ping"))
	require.Nil(t, msg)
	require.Error(t, err)
}

func TestFirstReplyReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	msg, err := client.FirstReply("bad subject", []byte("ping"))
	require.Nil(t, msg)
	require.Error(t, err)
}

func TestFirstReplyProtoReturnsErrorForNilRequest(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	msg, err := client.FirstReplyProto("test.first.reply.nil", nil)
	require.Nil(t, msg)
	require.Error(t, err)
}

func TestFirstReplyProtoReturnsErrorForInvalidSubject(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	msg, err := client.FirstReplyProto("bad subject", &pb.MicroVMLaunchRequest{})
	require.Nil(t, msg)
	require.Error(t, err)
}

func TestFirstReplyProtoReturnsErrorForMarshalFailure(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)

	msg, err := client.FirstReplyProto("test.first.reply.marshal", failingProtoMessage{})
	require.Nil(t, msg)
	require.Error(t, err)
}

func TestSubscribeDeliversMessage(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	delivered := make(chan *natsgo.Msg, 1)
	_, err = client.Subscribe("test.subscribe", func(msg *natsgo.Msg) error {
		delivered <- msg
		return nil
	})
	require.NoError(t, err)

	require.NoError(t, rawConn.Publish("test.subscribe", []byte("hello subscribe")))
	require.NoError(t, rawConn.Flush())

	select {
	case msg := <-delivered:
		require.Equal(t, "test.subscribe", msg.Subject)
		require.Equal(t, []byte("hello subscribe"), msg.Data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription delivery")
	}
}

func TestFlushWaitsForPendingOperations(t *testing.T) {
	url := startTestNATSServer(t)
	client := newTestClient(t, url)
	rawConn, err := natsgo.Connect(url)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawConn.Close()
	})

	subscriber, err := rawConn.SubscribeSync("test.flush")
	require.NoError(t, err)
	require.NoError(t, rawConn.Flush())

	require.NoError(t, client.Publish("test.flush", []byte("flush me")))
	require.NoError(t, client.Flush(5*time.Second))

	msg, err := subscriber.NextMsg(time.Second)
	require.NoError(t, err)
	require.Equal(t, []byte("flush me"), msg.Data)
}

func TestDrainClosesConnection(t *testing.T) {
	buf := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	url := startTestNATSServer(t)
	client := newTestClientWithLogger(t, url, logger)

	require.NoError(t, client.Drain())
	require.Eventually(t, client.conn.IsClosed, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "nats closed")
	}, time.Second, 10*time.Millisecond)
}

func TestCloseClosesConnection(t *testing.T) {
	buf := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	url := startTestNATSServer(t)
	client := newTestClientWithLogger(t, url, logger)

	client.Close()
	require.True(t, client.conn.IsClosed())
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "nats closed")
	}, time.Second, 10*time.Millisecond)
}

func TestUnmarshalProto(t *testing.T) {
	reply := &pb.MicroVMLaunchResponse{
		Accepted:     true,
		ErrorMessage: "unmarshal-ok",
	}
	payload, err := proto.Marshal(reply)
	require.NoError(t, err)

	msg := &natsgo.Msg{Subject: "test.unmarshal", Data: payload}

	var got pb.MicroVMLaunchResponse
	require.NoError(t, UnmarshalProto(msg, &got))
	require.True(t, got.GetAccepted())
	require.Equal(t, "unmarshal-ok", got.GetErrorMessage())
}
