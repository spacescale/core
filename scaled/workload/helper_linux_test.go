package workload

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/spacescale/core/shared/nats"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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
func newTestClient(t *testing.T, url string) *nats.Client {
	t.Helper()
	client, err := nats.New(url, "test", newTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func capturePublishedMsg(t *testing.T, client *nats.Client) <-chan *nats.Msg {
	t.Helper()

	replies := make(chan *nats.Msg, 1)
	_, err := client.Subscribe("reply.subject", func(msg *nats.Msg) error {
		replies <- msg
		return nil
	})
	require.NoError(t, err)

	return replies
}

func receivePublishedMsg(t *testing.T, replies <-chan *nats.Msg) *nats.Msg {
	t.Helper()

	select {
	case msg := <-replies:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for nats reply")
		return nil
	}
}
