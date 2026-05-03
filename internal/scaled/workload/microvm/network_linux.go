// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

//go:build linux

package microvm

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

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
