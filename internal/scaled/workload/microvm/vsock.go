package microvm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

const (
	// CID 2 is the Linux vsock host; guests start at 3.
	firstGuestCID uint32 = 3
	lastGuestCID  uint32 = ^uint32(0)
)

const (
	controlPort uint32 = 10000
	logPort     uint32 = 10001
)

const (
	controlFrameHeaderSize      = 10
	controlFrameVersion    byte = 1
	controlFrameKindHello  byte = 1
)

const acceptDeadline = 250 * time.Millisecond

var (
	errNoGuestCIDAvailable = errors.New("no guest vsock cid available")
	controlFrameMagic      = [4]byte{'S', 'C', 'D', 'T'}
)

// VSockListeners owns the host-side Unix socket listeners behind one
// Firecracker vsock device.
type VSockListeners struct {
	BasePath    string
	ControlPath string
	LogPath     string

	control *net.UnixListener
	log     *net.UnixListener
}

// cidAllocator tracks guest vsock CIDs for this scaled process.
type cidAllocator struct {
	mu   sync.Mutex
	next uint32
	used map[uint32]struct{}
}

type controlFrameHeader struct {
	Magic         [4]byte
	Version       byte
	Kind          byte
	PayloadLength uint32
}

func newCIDAllocator() *cidAllocator {
	return &cidAllocator{
		next: firstGuestCID,
		used: make(map[uint32]struct{}),
	}
}

func (a *cidAllocator) Acquire() (uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	start := a.next
	for {
		cid := a.next
		if cid < firstGuestCID {
			cid = firstGuestCID
			a.next = cid
		}

		if _, exists := a.used[cid]; !exists {
			a.used[cid] = struct{}{}
			a.advanceLocked()
			return cid, nil
		}

		a.advanceLocked()
		if a.next == start {
			return 0, errNoGuestCIDAvailable
		}
	}
}

func (a *cidAllocator) Release(cid uint32) {
	if cid < firstGuestCID {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.used, cid)
	if cid < a.next {
		a.next = cid
	}
}

func (a *cidAllocator) advanceLocked() {
	if a.next == lastGuestCID {
		a.next = firstGuestCID
		return
	}

	a.next++
	if a.next < firstGuestCID {
		a.next = firstGuestCID
	}
}

// openVSockListeners creates the control and log listeners before the VM boots.
// Firecracker maps guest connections to host sockets named <base>_<port>.
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

// WaitForHello proves the guest reached scoutd userspace on the control channel.
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

// AcceptLog waits for the guest raw log stream. The launcher writes it to disk.
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

// Close tears down both Unix listeners and removes their socket files.
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

func vsockPortPath(basePath string, port uint32) string {
	return fmt.Sprintf("%s_%d", basePath, port)
}

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

func expectHelloFrame(r io.Reader) error {
	header, err := readControlFrameHeader(r)
	if err != nil {
		return err
	}
	if header.Magic != controlFrameMagic {
		return fmt.Errorf("invalid control frame magic: got %q", header.Magic)
	}
	if header.Version != controlFrameVersion {
		return fmt.Errorf("unsupported control frame version: got %d", header.Version)
	}
	if header.Kind != controlFrameKindHello {
		return fmt.Errorf("unexpected control frame kind: got %d", header.Kind)
	}
	if header.PayloadLength != 0 {
		return fmt.Errorf("hello frame must not carry payload: got %d bytes", header.PayloadLength)
	}
	return nil
}

func readControlFrameHeader(r io.Reader) (controlFrameHeader, error) {
	var raw [controlFrameHeaderSize]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return controlFrameHeader{}, fmt.Errorf("read control frame header: %w", err)
	}

	var header controlFrameHeader
	copy(header.Magic[:], raw[0:4])
	header.Version = raw[4]
	header.Kind = raw[5]
	header.PayloadLength = binary.BigEndian.Uint32(raw[6:10])
	return header, nil
}
