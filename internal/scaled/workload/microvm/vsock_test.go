package microvm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVSockPortPathUsesFirecrackerConvention(t *testing.T) {
	path := vsockPortPath("/tmp/v.sock", 10000)
	require.Equal(t, "/tmp/v.sock_10000", path)
}

func TestCIDAllocatorStartsAtThree(t *testing.T) {
	a := newCIDAllocator()
	cid, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), cid)
}

func TestCIDAllocatorReusesReleasedCID(t *testing.T) {
	a := newCIDAllocator()
	first, err := a.Acquire()
	require.NoError(t, err)
	second, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), first)
	require.Equal(t, uint32(4), second)
	a.Release(first)
	reused, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), reused)
}

func TestCIDAllocatorIgnoresReservedReleaseValues(t *testing.T) {
	a := newCIDAllocator()
	a.Release(0)
	a.Release(1)
	a.Release(2)
	cid, err := a.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint32(3), cid)
}

func TestExpectHelloFrame(t *testing.T) {
	err := expectHelloFrame(bytes.NewReader(validHelloFrame()))
	require.NoError(t, err)
}

func TestExpectHelloFrameRejectsWrongMagic(t *testing.T) {
	raw := validHelloFrame()
	copy(raw[0:4], []byte("NOPE"))

	err := expectHelloFrame(bytes.NewReader(raw))
	require.Error(t, err)
}

func TestExpectHelloFrameRejectsWrongKind(t *testing.T) {
	raw := validHelloFrame()
	raw[5] = 99

	err := expectHelloFrame(bytes.NewReader(raw))
	require.Error(t, err)
}

func TestExpectHelloFrameRejectsPayload(t *testing.T) {
	raw := validHelloFrame()
	binary.BigEndian.PutUint32(raw[6:10], 4)

	err := expectHelloFrame(bytes.NewReader(raw))
	require.Error(t, err)
}

func TestListenUnixRemovesStalePathBeforeListening(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o644))

	listener, err := listenUnix(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
	})

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestOpenVSockListenersCreatesControlAndLogSockets(t *testing.T) {
	workspace := Workspace{
		JailerRootDir: filepath.Join(shortTempDir(t), "root"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(workspace.VSockHostPath()), 0o755))

	listeners, err := openVSockListeners(workspace)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listeners.Close()
	})

	require.Equal(t, workspace.VSockHostPath(), listeners.BasePath)
	require.Equal(t, workspace.VSockHostPath()+"_10000", listeners.ControlPath)
	require.Equal(t, workspace.VSockHostPath()+"_10001", listeners.LogPath)
	require.NotNil(t, listeners.control)
	require.NotNil(t, listeners.log)

	controlInfo, err := os.Stat(listeners.ControlPath)
	require.NoError(t, err)
	require.NotZero(t, controlInfo.Mode()&os.ModeSocket)

	logInfo, err := os.Stat(listeners.LogPath)
	require.NoError(t, err)
	require.NotZero(t, logInfo.Mode()&os.ModeSocket)
}

func TestVSockListenersCloseRemovesSocketFiles(t *testing.T) {
	workspace := Workspace{
		JailerRootDir: filepath.Join(shortTempDir(t), "root"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(workspace.VSockHostPath()), 0o755))

	listeners, err := openVSockListeners(workspace)
	require.NoError(t, err)

	require.NoError(t, listeners.Close())

	_, err = os.Stat(listeners.ControlPath)
	require.True(t, os.IsNotExist(err))

	_, err = os.Stat(listeners.LogPath)
	require.True(t, os.IsNotExist(err))
}

func TestAcceptUnixReturnsAcceptedConnection(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "accept.sock")
	listener, err := listenUnix(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
	})

	clientCh := make(chan *net.UnixConn, 1)
	errCh := make(chan error, 1)
	go dialUnix(path, clientCh, errCh)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	serverConn, err := acceptUnix(ctx, listener)
	require.NoError(t, err)
	defer serverConn.Close()

	clientConn := requireClientConn(t, clientCh, errCh)
	defer clientConn.Close()
}

func TestAcceptUnixStopsWhenContextIsCanceled(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "cancel.sock")
	listener, err := listenUnix(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = acceptUnix(ctx, listener)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestWaitForHelloAcceptsValidControlFrame(t *testing.T) {
	workspace := Workspace{
		JailerRootDir: filepath.Join(shortTempDir(t), "root"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(workspace.VSockHostPath()), 0o755))

	listeners, err := openVSockListeners(workspace)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listeners.Close()
	})

	errCh := make(chan error, 1)
	go func() {
		conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: listeners.ControlPath, Net: "unix"})
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		_, err = conn.Write(validHelloFrame())
		errCh <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, listeners.WaitForHello(ctx))
	require.NoError(t, requireAsyncErr(t, errCh))
}

func TestAcceptLogReturnsLogConnection(t *testing.T) {
	workspace := Workspace{
		JailerRootDir: filepath.Join(shortTempDir(t), "root"),
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(workspace.VSockHostPath()), 0o755))

	listeners, err := openVSockListeners(workspace)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listeners.Close()
	})

	clientCh := make(chan *net.UnixConn, 1)
	errCh := make(chan error, 1)
	go dialUnix(listeners.LogPath, clientCh, errCh)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	serverConn, err := listeners.AcceptLog(ctx)
	require.NoError(t, err)
	defer serverConn.Close()

	clientConn := requireClientConn(t, clientCh, errCh)
	defer clientConn.Close()
}

func validHelloFrame() []byte {
	raw := make([]byte, controlFrameHeaderSize)
	copy(raw[0:4], controlFrameMagic[:])
	raw[4] = controlFrameVersion
	raw[5] = controlFrameKindHello
	binary.BigEndian.PutUint32(raw[6:10], 0)
	return raw
}

func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "vsock-test-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return dir
}

func dialUnix(path string, connCh chan<- *net.UnixConn, errCh chan<- error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		errCh <- err
		return
	}
	connCh <- conn
}

func requireClientConn(t *testing.T, connCh <-chan *net.UnixConn, errCh <-chan error) *net.UnixConn {
	t.Helper()

	select {
	case conn := <-connCh:
		return conn
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Unix client connection")
	}

	return nil
}

func requireAsyncErr(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async Unix socket operation")
	}

	return nil
}
