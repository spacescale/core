package microvm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

const (
	// scoutd connects to these two fixed host ports from inside the guest.
	//
	// Control is the structured machine-readable channel.
	// Log is the raw byte stream channel.
	controlPort uint32 = 10000
	logPort     uint32 = 10001

	// We use a short rolling deadline so Accept can still respond to context
	// cancellation without forcing us into a more complicated goroutine or poll loop.
	acceptDeadline = 250 * time.Millisecond
)

// VSockListeners owns the host-side Unix socket listeners that back the
// Firecracker vsock device for one microVM.
//
// Firecracker does not expose TCP listeners here. Instead, when the guest
// connects to host CID 2 and a given port, the host side maps that traffic onto
// Unix sockets derived from Workspace.VSockHostPath().
//
// If the base path is:
//
//	/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/<microvm-id>/root/v.sock
//
// Then the host must listen on:
//
//	/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/<microvm-id>/root/v.sock_10000
//	/var/lib/spacescale/j/firecracker-v1.15.1-x86_64/<microvm-id>/root/v.sock_10001
//
// for scoutd's control and log channels.
type VSockListeners struct {
	BasePath    string
	ControlPath string
	LogPath     string

	control *net.UnixListener
	log     *net.UnixListener
}

// openVSockListeners creates both host-side Unix listeners needed before the VM boots.
//
// Guest-initiated vsock connections use Firecracker's <base>_<port> Unix socket
// convention, so scaled must listen on v.sock_10000 and v.sock_10001 before
// scoutd tries to connect.
func openVSockListeners(workspace Workspace) (*VSockListeners, error) {
	controlPath := vsockPortPath(workspace.VSockHostPath(), controlPort)
	logPath := vsockPortPath(workspace.VSockHostPath(), logPort)

	controlListener, err := listenUnix(controlPath)
	if err != nil {
		return nil, fmt.Errorf("listen control vsock socket: %w", err)
	}

	logListener, err := listenUnix(logPath)
	if err != nil {
		_ = controlListener.Close()
		_ = os.Remove(controlPath)
		return nil, fmt.Errorf("listen log vsock socket: %w", err)
	}

	return &VSockListeners{
		BasePath:    workspace.VSockHostPath(),
		ControlPath: controlPath,
		LogPath:     logPath,
		control:     controlListener,
		log:         logListener,
	}, nil
}

// vsockPortPath converts the Firecracker vsock base path into the concrete host
// Unix socket path for one port.
//
// Firecracker's host-side convention is:
//
//	<base>_<port>
func vsockPortPath(basePath string, port uint32) string {
	return fmt.Sprintf("%s_%d", basePath, port)
}

// listenUnix creates one Unix domain listener for a Firecracker host-side vsock port.
//
// We remove any stale socket file first because an unclean previous exit can leave
// the old path behind and prevent the next boot from binding cleanly.
func listenUnix(path string) (*net.UnixListener, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}

	addr := &net.UnixAddr{Name: path, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}

	return listener, nil
}

// Close tears down both Unix listeners and removes their socket files from disk.
//
// This is the right cleanup path for launch failure and later for terminal VM teardown.
func (v *VSockListeners) Close() error {
	if v == nil {
		return nil
	}

	var errs []error

	if v.control != nil {
		if err := v.control.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close control listener: %w", err))
		}
	}

	if v.log != nil {
		if err := v.log.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close log listener: %w", err))
		}
	}

	if v.ControlPath != "" {
		if err := os.Remove(v.ControlPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove control socket: %w", err))
		}
	}

	if v.LogPath != "" {
		if err := os.Remove(v.LogPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove log socket: %w", err))
		}
	}

	return errors.Join(errs...)
}

// WaitForHello blocks until scoutd connects on the control channel and sends the
// expected hello frame.
// If this returns nil, we know the kernel booted, the rootfs mounted, scoutd is
// alive as PID 1, and the host-guest vsock path is working.
func (v *VSockListeners) WaitForHello(ctx context.Context) error {
	if v == nil || v.control == nil {
		return fmt.Errorf("control listener is not initialized")
	}

	conn, err := acceptUnix(ctx, v.control)
	if err != nil {
		return fmt.Errorf("accept control connection: %w", err)
	}
	defer conn.Close()

	if err := expectHelloFrame(conn); err != nil {
		return fmt.Errorf("read hello frame: %w", err)
	}

	return nil
}

// AcceptLog waits for the guest to connect the raw log channel.
//
// We do not need to consume logs yet to satisfy the first hello boot milestone,
// but exposing this now keeps the ownership of log listener acceptance inside the
// vsock boundary instead of scattering it later.
func (v *VSockListeners) AcceptLog(ctx context.Context) (*net.UnixConn, error) {
	if v == nil || v.log == nil {
		return nil, fmt.Errorf("log listener is not initialized")
	}
	conn, err := acceptUnix(ctx, v.log)
	if err != nil {
		return nil, fmt.Errorf("accept log connection: %w", err)
	}
	return conn, nil
}

// acceptUnix waits for a Unix listener to accept one connection while still
// respecting context cancellation.
//
// net.UnixListener does not accept a context directly, so we use a short rolling
// deadline and retry on timeout until the caller's context is done.
func acceptUnix(ctx context.Context, listener *net.UnixListener) (*net.UnixConn, error) {
	for {
		if err := listener.SetDeadline(time.Now().Add(acceptDeadline)); err != nil {
			return nil, fmt.Errorf("set accept deadline: %w", err)
		}

		conn, err := listener.AcceptUnix()
		if err == nil {
			return conn, nil
		}

		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				continue
			}
		}

		return nil, err
	}
}
