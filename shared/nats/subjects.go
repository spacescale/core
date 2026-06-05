// Package nats defines the shared NATS messaging taxonomy.
//
// It is the central registry for NATS subjects used by the stateless
// Control Plane (scalecp) and the autonomous Edge daemons (scaled).
//
// Architecture: The Decentralized Auction
//
// SpaceScale avoids centralized, database-locked scheduling. Instead, it relies
// on an event-driven Request-Reply pattern. The control plane broadcasts one
// resolved microvm shape, and edge nodes independently calculate their real
// time physical capacity to bid on that workload.
package nats

import "fmt"

// Subjects, the NAT routing addresses
const (
	// subjectNodeAuction is the regional broadcast frequency. CP publishes to this subject to solicit bids for new microvms.
	// %s must be formatted with the target region  e,g, us-east
	subjectNodeAuction = "node.auction.%s.microvm"

	// subjectNodeMicroVMLaunch is the personal inbox for a specific edge daemon. CP publishes launch intent after a node wins an auction.
	// The %s must be formatted with the target node's boot_id.
	subjectNodeMicroVMLaunch = "node.cmd.%s.microvm.launch"
)

// NodeAuctionSubject generates the regional broadcast frequency for placement auctions.
//
// Example: "node.auction.us-east.microvm"
func NodeAuctionSubject(region string) string {
	return fmt.Sprintf(subjectNodeAuction, region)
}

// NodeMicroVMLaunchSubject generates the specific inbox for a node to receive
// its launch commands after winning an auction.
//
// Example: "node.cmd.boot-12345.microvm.launch"
func NodeMicroVMLaunchSubject(bootID string) string {
	return fmt.Sprintf(subjectNodeMicroVMLaunch, bootID)
}
