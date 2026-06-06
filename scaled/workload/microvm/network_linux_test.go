//go:build linux

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
