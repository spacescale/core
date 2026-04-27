package microvm

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpectHelloFrame(t *testing.T) {
	var raw [controlFrameHeaderSize]byte
	copy(raw[0:4], controlFrameMagic[:])
	raw[4] = controlFrameVersion
	raw[5] = controlFrameKindHello
	binary.BigEndian.PutUint32(raw[6:10], 0)

	// we read from pre allocated array using new reader
	err := expectHelloFrame(bytes.NewReader(raw[:]))
	require.NoError(t, err)
}

func TestExpectHelloFrameRejectsWrongMagic(t *testing.T) {
	var raw [controlFrameHeaderSize]byte
	copy(raw[0:4], []byte("NOPE"))
	raw[4] = controlFrameVersion
	raw[5] = controlFrameKindHello
	binary.BigEndian.PutUint32(raw[6:10], 0)
	err := expectHelloFrame(bytes.NewReader(raw[:]))
	require.Error(t, err)
}

func TestExpectHelloFrameRejectsWrongKind(t *testing.T) {
	var raw [controlFrameHeaderSize]byte
	copy(raw[0:4], controlFrameMagic[:])
	raw[4] = controlFrameVersion
	raw[5] = 99
	binary.BigEndian.PutUint32(raw[6:10], 0)
	err := expectHelloFrame(bytes.NewReader(raw[:]))
	require.Error(t, err)
}

func TestExpectHelloFrameRejectsPayload(t *testing.T) {
	var raw [controlFrameHeaderSize]byte
	copy(raw[0:4], controlFrameMagic[:])
	raw[4] = controlFrameVersion
	raw[5] = controlFrameKindHello
	binary.BigEndian.PutUint32(raw[6:10], 4)
	err := expectHelloFrame(bytes.NewReader(raw[:]))
	require.Error(t, err)
}
