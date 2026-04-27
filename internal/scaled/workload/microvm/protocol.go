package microvm

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// The scoutd control protocol starts every frame with a fixed 10 byte header.
	//
	// Layout
	// 4 bytes magic
	// 1 byte protocol version
	// 1 byte frame kind
	// 4 bytes payload length in big endian order
	controlFrameHeaderSize = 10

	// This is the current control protocol version used by scoutd.
	controlFrameVersion byte = 1

	// Kind 1 is the first success signal we care about for the hello boot issue.
	controlFrameKindHello byte = 1
)

// controlFrameMagic is the 4-byte protocol signature ("SCDT" for SCoutD Tether)
// that MUST strictly prefix every binary control frame sent over the virtio-vsock boundary.
//
// Because virtio-vsock operates as a raw byte stream, this magic number acts as our
// deterministic synchronization marker between the guest bootloader (Zig) and the
// host daemon (Go). If scaled attempts to read a frame header and these first 4 bytes
// do not match perfectly, it indicates stream corruption, a compromised guest, or a
// kernel panic writing garbage to the socket.
//
// In the event of a mismatch, scaled will immediately abort the parse and sever
// the connection, protecting the host control plane from malicious or malformed payloads.
var controlFrameMagic = [4]byte{'S', 'C', 'D', 'T'}

// controlFrameHeader is the small fixed header that scoutd writes first on the
// control vsock channel.
//
// We keep this as a tiny explicit struct instead of using a more generic framing
// abstraction because the first Firecracker milestone only needs to validate one
// very specific event from the guest.
//
// The event we care about is hello.
// If we can read a valid hello frame, we know the guest booted far enough for
// the kernel, rootfs, PID 1, and vsock path to all be working together.
type controlFrameHeader struct {
	Magic         [4]byte
	Version       byte
	Kind          byte
	PayloadLength uint32
}

// readControlFrameHeader pulls the fixed size header off the control stream and
// decodes it into a structured value.
//
// This does not decide whether the frame is valid for our boot milestone yet.
// It only does the mechanical decode step.
func readControlFrameHeader(r io.Reader) (controlFrameHeader, error) {
	var raw [controlFrameHeaderSize]byte

	// io.ReadFull blocks until it receives EXACTLY controlFrameHeaderSize bytes.
	// If the guest sends 9 bytes, it waits. If the connection drops at 9 bytes,
	// it throws an error. We refuse to parse fragmented or incomplete headers.
	// raw[:] creates a temporary slice pointing to our fixed array so ReadFull can use it.
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return controlFrameHeader{}, fmt.Errorf("read control frame header: %w", err)
	}

	// Now that we have caught exactly 10 bytes
	// we map those raw bytes into our structured Go struct.
	var header controlFrameHeader

	// Bytes 0, 1, 2, 3 -> The Magic Number . "SCDT"
	// We copy the first 4 bytes directly into the struct's Magic array.
	copy(header.Magic[:], raw[0:4])

	// Byte 4 -> Protocol Version
	header.Version = raw[4]

	// Byte 5 -> Message Kind (e.g., 0x01 for HELLO, 0x02 for LOG)
	header.Kind = raw[5]

	// Bytes 6, 7, 8, 9 -> Payload Length
	// We take the last 4 bytes and mathematically convert them into a 32-bit integer.
	// We MUST use BigEndian (Network Byte Order) so that even if the guest is
	// ARM64 and the host is AMD64, they agree on how to read the number.
	header.PayloadLength = binary.BigEndian.Uint32(raw[6:10])

	// The envelope is successfully parsed.
	return header, nil
}

// expectHelloFrame validates that the next frame on the control channel is the
// exact scoutd hello frame we expect for the first real boot milestone.
//
// A valid hello means:
// magic is SCDT
// protocol version is 1
// frame kind is hello
// payload length is zero
//
// We keep this very strict because the whole point of this issue is to treat
// hello as the first clean guest boot proof.
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
