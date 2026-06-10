//go:build linux

package microvm

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
)

const (
	guestIPv4CIDR = "172.16.0.2/30"
	hostIPv4CIDR  = "172.16.0.1/30"
	mmdsIPv4      = "169.254.169.254"
)

// Network describes one prepared host-side microVM network attachment.
type Network struct {
	TapName   string
	GuestMAC  string
	HostCIDR  string
	GuestCIDR string
	MMDSIP    net.IP
}

func tapNameForMicroVM(microvmID string) string {
	id := strings.ToLower(strings.ReplaceAll(microvmID, "-", ""))
	if len(id) > 11 {
		id = id[:11]
	}
	return "tap" + id
}

func guestMACForMicroVM(microvmID string) string {
	sum := sha256.Sum256([]byte(microvmID))
	mac := net.HardwareAddr{0x02, 0x53, sum[0], sum[1], sum[2], sum[3]}
	return mac.String()
}

func prepareNetwork(microvmID string, uid int, gid int) (*Network, error) {
	tapName := tapNameForMicroVM(microvmID)
	network := &Network{
		TapName:   tapName,
		GuestMAC:  guestMACForMicroVM(microvmID),
		HostCIDR:  hostIPv4CIDR,
		GuestCIDR: guestIPv4CIDR,
		MMDSIP:    net.ParseIP(mmdsIPv4).To4(),
	}

	if err := deleteTapIfExists(tapName); err != nil {
		return nil, err
	}

	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{Name: tapName},
		Mode:      netlink.TUNTAP_MODE_TAP,
		Owner:     uint32(uid),
		Group:     uint32(gid),
	}
	if err := netlink.LinkAdd(tap); err != nil {
		return nil, fmt.Errorf("create tap %s: %w", tapName, err)
	}

	prepared := false
	defer func() {
		if !prepared {
			_ = network.Cleanup()
		}
	}()

	link, err := netlink.LinkByName(tapName)
	if err != nil {
		return nil, fmt.Errorf("lookup tap %s: %w", tapName, err)
	}

	hostAddr, err := hostNetworkAddr()
	if err != nil {
		return nil, err
	}
	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: hostAddr}); err != nil {
		return nil, fmt.Errorf("assign host address to tap %s: %w", tapName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("bring tap %s up: %w", tapName, err)
	}

	prepared = true
	return network, nil
}

// Cleanup removes the host TAP device associated with this microVM network.
func (n *Network) Cleanup() error {
	if n == nil {
		return nil
	}

	return deleteTapIfExists(n.TapName)
}

func deleteTapIfExists(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		if _, ok := errors.AsType[netlink.LinkNotFoundError](err); ok {
			return nil
		}
		return fmt.Errorf("lookup tap %s: %w", name, err)
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("delete tap %s: %w", name, err)
	}

	return nil
}

func hostNetworkAddr() (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(hostIPv4CIDR)
	if err != nil {
		return nil, fmt.Errorf("parse host address: %w", err)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("host address is not IPv4: %s", hostIPv4CIDR)
	}

	ipNet.IP = ip4
	return ipNet, nil
}
