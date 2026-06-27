//go:build linux

// network_linux_test covers the pure, deterministic helpers in this file.
//
// Intentionally not unit tested here:
//   - prepareNetwork
//   - deleteTapIfExists
//   - (*Network).Cleanup via real TAP device mutation
//
// Reason: those paths depend on privileged netlink operations and real host TAP
// device state. The unit tests here stay focused on the stable naming, IP
// derivation, and subnet allocation logic without introducing fake netlink seams
// into the concrete microvm network path.
package microvm

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTapNameForMicroVMFitsLinuxLimit(t *testing.T) {
	const linuxInterfaceNameMax = 15
	const microvmID = "3534e563-198e-4f8a-8146-cab912e4b996"

	name := tapNameForMicroVM(microvmID)

	require.Equal(t, "tap3534e563198", name)
	require.LessOrEqual(t, len(name), linuxInterfaceNameMax)
	require.True(t, strings.HasPrefix(name, "tap"))
	require.NotContains(t, name, "-")
	require.Equal(t, name, tapNameForMicroVM(microvmID))
}

func TestGuestMACForMicroVMIsDeterministicLocalUnicast(t *testing.T) {
	const microvmID = "3534e563-198e-4f8a-8146-cab912e4b996"

	mac := guestMACForMicroVM(microvmID)

	require.Equal(t, mac, guestMACForMicroVM(microvmID))

	hardwareAddr, err := net.ParseMAC(mac)
	require.NoError(t, err)
	require.Len(t, hardwareAddr, 6)
	require.Equal(t, byte(0x02), hardwareAddr[0]&0x02)
	require.Equal(t, byte(0x00), hardwareAddr[0]&0x01)
	require.Equal(t, byte(0x53), hardwareAddr[1])
}

func TestSubnetForIndexZero(t *testing.T) {
	s := subnetForIndex(0)

	require.Equal(t, uint16(0), s.Index)
	require.Equal(t, "172.16.0.1/30", s.HostCIDR)
	require.Equal(t, "172.16.0.2/30", s.GuestCIDR)
	require.Equal(t, net.ParseIP("172.16.0.1").To4(), s.HostIP)
	require.Equal(t, net.ParseIP("172.16.0.2").To4(), s.GuestIP)
}

func TestSubnetForIndexOne(t *testing.T) {
	s := subnetForIndex(1)

	require.Equal(t, "172.16.0.5/30", s.HostCIDR)
	require.Equal(t, "172.16.0.6/30", s.GuestCIDR)
	require.Equal(t, net.ParseIP("172.16.0.5").To4(), s.HostIP)
	require.Equal(t, net.ParseIP("172.16.0.6").To4(), s.GuestIP)
}

func TestSubnetForIndexCrossesOctetBoundary(t *testing.T) {
	// Index 64: offset = 256, so third octet rolls to 1.
	s := subnetForIndex(64)

	require.Equal(t, "172.16.1.1/30", s.HostCIDR)
	require.Equal(t, "172.16.1.2/30", s.GuestCIDR)
}

func TestSubnetForIndexMax(t *testing.T) {
	s := subnetForIndex(maxSubnetIndex)

	require.Equal(t, "172.16.255.253/30", s.HostCIDR)
	require.Equal(t, "172.16.255.254/30", s.GuestCIDR)
}

func TestSubnetAllocatorAcquiresSequentially(t *testing.T) {
	alloc := newSubnetAllocator()

	s0, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint16(0), s0.Index)

	s1, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint16(1), s1.Index)

	s2, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint16(2), s2.Index)
}

func TestSubnetAllocatorReleaseReusesIndex(t *testing.T) {
	alloc := newSubnetAllocator()

	s0, _ := alloc.Acquire()
	_, _ = alloc.Acquire()

	alloc.Release(s0.Index)

	reused, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, s0.Index, reused.Index)
}

func TestSubnetAllocatorWrapsAround(t *testing.T) {
	alloc := newSubnetAllocator()
	alloc.next = maxSubnetIndex

	s, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, maxSubnetIndex, s.Index)

	// Next acquire wraps to 0.
	s2, err := alloc.Acquire()
	require.NoError(t, err)
	require.Equal(t, uint16(0), s2.Index)
}

func TestSubnetAllocatorExhaustion(t *testing.T) {
	alloc := newSubnetAllocator()
	// Simulate full: mark all as used.
	for i := uint16(0); i <= maxSubnetIndex; i++ {
		alloc.used[i] = struct{}{}
	}

	_, err := alloc.Acquire()
	require.ErrorIs(t, err, errNoSubnetAvailable)
}

func TestParseNetworkAddr(t *testing.T) {
	addr, err := parseNetworkAddr("172.16.0.5/30")
	require.NoError(t, err)
	require.Equal(t, net.ParseIP("172.16.0.5").To4(), addr.IP)

	ones, bits := addr.Mask.Size()
	require.Equal(t, 30, ones)
	require.Equal(t, 32, bits)
}

func TestNetworkGatewayIP(t *testing.T) {
	n := &Network{HostCIDR: "172.16.0.5/30"}
	require.Equal(t, net.ParseIP("172.16.0.5").To4(), n.gatewayIP())
}

func TestBuildGuestdKernelArgs(t *testing.T) {
	args := buildGuestdKernelArgs("172.16.0.6/30", net.ParseIP("172.16.0.5").To4())

	require.Contains(t, args, "guestd.ipv4=172.16.0.6/30")
	require.Contains(t, args, "guestd.gateway=172.16.0.5")
	require.Contains(t, args, "guestd.mmds=169.254.169.254")
	require.Contains(t, args, "ro root=/dev/vda")
}
