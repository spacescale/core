//go:build linux

package microvm

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/vishvananda/netlink"
)

const mmdsIPv4 = "169.254.169.254"

// Subnet Pool: 172.16.0.0/16 carved into /30 point-to-point links
// Each /30 yields exactly 2 usable addresses (Host and Guest).
// 65, 536 addresses /4 per /30 = 16/384 subnets per host.
const (
	subnetBaseA      byte   = 172
	subnetBaseB      byte   = 16
	maxSubnetIndex   uint16 = 16383 // (65536 /4) - 1
	firstSubnetIndex uint16 = 0
)

var errNoSubnetAvailable = errors.New("no vm subnet available")

// Subnet is a computed /30 point-to-point link for one VM.
type Subnet struct {
	Index     uint16
	HostCIDR  string // e.g. "172.16.0.1/30"
	GuestCIDR string // e.g. "172.16.0.2/30"
	HostIP    net.IP // e.g. 172.16.0.1
	GuestIP   net.IP // e.g. 172.16.0.2
}

// Network describes one prepared host-side microVM network attachment.
type Network struct {
	TapName     string
	GuestMAC    string
	HostCIDR    string
	GuestCIDR   string
	MMDSIP      net.IP
	SubnetIndex uint16
}

// subnetAllocator tracks per-VM /30 subnet leases for this scaled process.
type subnetAllocator struct {
	mu   sync.Mutex
	next uint16
	used map[uint16]struct{}
}

// newSubnetAllocator creates and returns a fresh subnetAllocator ready to lease
// /30 point-to-point subnets from the 172.16.0.0/16 pool.
//
// The returned allocator starts scanning from firstSubnetIndex (0) and has no
// subnets marked as used. Each subsequent lease carves the next available /30
// block from the pool; a block is considered available when its index does not
// appear in the used set. Up to maxSubnetIndex+1 (16,384) distinct /30 subnets
// can be handed out before the pool is exhausted, at which point Acquire
// returns errNoSubnetAvailable.
//
// The caller is expected to keep a single allocator for the lifetime of the
// scaled process and share it across all microVM creations on this host so
// that subnet indices are never double-assigned. The allocator is safe for
// concurrent use; all mutating operations take the embedded mutex.
//
// Each leased index maps to a concrete /30 via subnetForIndex:
//
//	index 0 → 172.16.0.0/30 (host .1, guest .2)
//	index 1 → 172.16.0.4/30 (host .5, guest .6)
//	index N → 172.16.X.Y/30 where offset = N*4
//
// Returned subnets are not released automatically; callers must explicitly
// return them to the pool when the owning microVM is destroyed so that the
// index can be reused by future leases.
func newSubnetAllocator() *subnetAllocator {
	return &subnetAllocator{
		next: firstSubnetIndex,
		used: make(map[uint16]struct{}),
	}
}

// Acquire leases the next available /30 subnet from the pool and reserves it
// against re-use until the matching Release is called. It is safe for
// concurrent use: the embedded mutex serializes every read and write of the
// used set and the next cursor, so two callers never receive the same index.
//
// The scan starts at the current next index. If that index is unused it is
// claimed on the spot; otherwise the cursor advances and the next candidate
// is inspected. advanceLocked wraps the cursor at maxSubnetIndex back to
// firstSubnetIndex, turning the pool into a ring so that indices freed below
// the cursor are revisited on the following revolution.
//
// Exhaustion is detected without a live counter: the starting index is saved
// before the loop, and if the cursor returns to it after a full sweep then
// every index has been tried and found occupied, so Acquire returns
// errNoSubnetAvailable. On success the claimed index is handed to
// subnetForIndex to produce the concrete Subnet, which is returned with nil.
//
// Callers must pair every successful Acquire with a Release to avoid leaks.
func (a *subnetAllocator) Acquire() (Subnet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	start := a.next
	for {
		// Try each candidate index in turn. If it is free, claim it, advance
		// the cursor past it, and return. If it is already in use, advance and
		// keep scanning; stop only when we come full circle back to start,
		// which means the entire pool is occupied.
		index := a.next
		if _, exists := a.used[index]; !exists {
			a.used[index] = struct{}{}
			a.advanceLocked()
			return subnetForIndex(index), nil
		}

		a.advanceLocked()
		if a.next == start {
			return Subnet{}, errNoSubnetAvailable
		}
	}
}

// Release returns a previously leased subnet index to the pool, making it
// eligible for re-use by a future Acquire. It is a no-op to Release an index
// that was never leased (or has already been released): delete on a missing
// map key is harmless, so callers do not need to track lease state themselves.
//
// Beyond clearing the used entry, Release also performs a low-water-mark
// rewind: when the freed index is strictly less than the current next cursor,
// the cursor is moved back down to that index. This means the just-freed
// index becomes the very next candidate Acquire will consider, so recently
// released subnets are re-handed out immediately rather than waiting for the
// ring to sweep all the way back around.
//
// The mutex is held for the whole operation, keeping the cursor and used set
// consistent with any concurrent Acquire. Releasing an index above the cursor
// leaves next unchanged; that index simply becomes available again whenever
// the scan naturally reaches it. Every Acquire must be paired with exactly
// one Release to avoid pool leaks.
func (a *subnetAllocator) Release(index uint16) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, index)
	if index < a.next {
		a.next = index
	}
}

// advanceLocked moves the next cursor forward by one position, wrapping from
// maxSubnetIndex back to firstSubnetIndex so the cursor cycles endlessly
// through the pool like a ring buffer. The "Locked" suffix is the usual Go
// convention signaling that the caller must already hold a.mu; Acquire and
// Release both call it from within their critical sections, so this helper
// never takes the mutex itself, which would deadlock.
//
// The wrap uses an explicit comparison against maxSubnetIndex rather than a
// modular increment (next = (next+1) % (maxSubnetIndex+1)) for two reasons:
// it avoids the per-call division that modulo would incur on uint16, and it
// keeps the boundary explicit and easy to audit. Because the cursor only ever
// moves by single steps and wraps at a known point, it can never skip an index
// or land outside the valid range.
//
// advanceLocked never inspects the used set, so advancing past a leased index
// is intentional and correct: Acquire is responsible for skipping occupied
// entries, while this helper merely moves the scan position. Callers outside
// the type should prefer Acquire/Release over invoking this directly.
func (a *subnetAllocator) advanceLocked() {
	if a.next == maxSubnetIndex {
		a.next = firstSubnetIndex
		return
	}
	a.next++
}

// subnetForIndex turns a subnet number into the actual IP addresses for one
// microVM's private /30 link in the 172.16.0.0/16 pool.
//
// Each /30 holds 4 addresses: the network address, the host, the guest, and the
// broadcast. Only the host (.1) and guest (.2) are usable — exactly the two a
// point-to-point link needs (one for the host TAP, one for the microVM). The
// pool splits into 65,536 / 4 = 16,384 subnets, numbered from 0.
//
// The address math is just counting. Subnet N starts at address N*4 (each
// subnet eats 4 addresses). That starting number is split across the last two
// octets of 172.16.X.Y: divide it by 256 to get X (which /24 block we are in),
// take the remainder to get Y (the position inside that block). Then:
//
//	host  = 172.16.X.(Y+1)
//	guest = 172.16.X.(Y+2)
//
// Examples:
//
//	index 0:  start 0    → 172.16.0.0/30   host .1   guest .2
//	index 1:  start 4    → 172.16.0.4/30   host .5   guest .6
//	index 63: start 252  → 172.16.0.252/30 host .253 guest .254
//	index 64: start 256  → 172.16.1.0/30   host .1   guest .2  (Y overflowed, X rolls up)
func subnetForIndex(index uint16) Subnet {
	offset := uint32(index) * 4
	hostIP := net.IPv4(subnetBaseA, subnetBaseB, byte(offset>>8), byte(offset&0xFF)+1)
	guestIP := net.IPv4(subnetBaseA, subnetBaseB, byte(offset>>8), byte(offset&0xFF)+2)

	return Subnet{
		Index:     index,
		HostCIDR:  hostIP.String() + "/30",
		GuestCIDR: guestIP.String() + "/30",
		HostIP:    hostIP.To4(),
		GuestIP:   guestIP.To4(),
	}

}

func tapNameForMicroVM(microvmID string) string {
	id := strings.ToLower(strings.ReplaceAll(microvmID, "-", ""))
	if len(id) > 11 {
		id = id[:11]
	}
	return "tap" + id
}

// guestMACForMicroVM derives a stable, locally-administered MAC address for a
// microVM from its identifier. The ID is hashed with SHA-256 and the first
// four bytes of the digest become the trailing octets, prefixed with the
// locally-administered OUI 02:53. Because the hash is deterministic, the same
// microvmID always yields the same MAC across process restarts and hosts.
//
// The leading byte 0x02 has the locally-administered bit (bit 1 of the first
// octet) set and the unicast/multicast a bit clear, marking the address as a
// self-assigned unicast MAC that will never clash with an IEEE-registered
// vendor OUI. The 02:53 prefix is a chosen sentinel that is easy to recognize
// in packet captures and arp tables as belonging to this scaled/microVM setup.
//
// Four bytes of hash give roughly four billion possible suffixes, so the
// chance of two microVMs colliding on the same MAC is negligible for any
// realistic fleet size. Collisions are also non-fatal: the guest MAC only
// needs to be unique on the local point-to-point /30 link, not globally.
// See tapNameForMicroVM for the matching host-side identifier derivation.
func guestMACForMicroVM(microvmID string) string {
	sum := sha256.Sum256([]byte(microvmID))
	mac := net.HardwareAddr{0x02, 0x53, sum[0], sum[1], sum[2], sum[3]}
	return mac.String()
}

// prepareNetwork creates and configures the host-side TAP device for a single
// microVM, returning a fully populated Network the caller can hand to the
// VMM. The tap name and guest MAC are derived deterministically from the
// microvmID, while the host/guest CIDRs come from the caller-supplied Subnet
// lease, so each VM gets its own isolated point-to-point /30 link.
//
// The function is idempotent in setup: any pre-existing tap with the same
// name is deleted first (cleaning up leftovers from a crashed previous run),
// then a fresh TUN/TAP is created owned by the given uid/gid so the VMM
// process can attach to it. The host address from the Subnet is assigned and
// the link is brought up.
//
// On any failure the deferred cleanup tears down the partially-built tap so
// no half-configured interface is left behind; only when every step succeeds
// is the prepared flag flipped, suppressing the cleanup. The returned Network
// carries the tap name, guest MAC, both CIDRs, and the MMDS metadata IP.
// The caller must eventually call Network.Cleanup to remove the tap on teardown.
func prepareNetwork(microvmID string, subnet Subnet, uid int, gid int) (*Network, error) {
	tapName := tapNameForMicroVM(microvmID)
	network := &Network{
		TapName:     tapName,
		GuestMAC:    guestMACForMicroVM(microvmID),
		HostCIDR:    subnet.HostCIDR,
		GuestCIDR:   subnet.GuestCIDR,
		MMDSIP:      net.ParseIP(mmdsIPv4).To4(),
		SubnetIndex: subnet.Index,
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

	hostAddr, err := parseNetworkAddr(subnet.HostCIDR)
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

func parseNetworkAddr(cidr string) (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse network address %s: %w", cidr, err)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("address is not IPv4: %s", cidr)
	}

	ipNet.IP = ip4
	return ipNet, nil
}
