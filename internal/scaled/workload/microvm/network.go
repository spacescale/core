// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

package microvm

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
)

const (
	guestIPv4CIDR = "172.16.0.2/30"
	hostIPv4CIDR  = "172.16.0.1/30"
	mmdsIPv4      = "169.254.169.254"
)

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
