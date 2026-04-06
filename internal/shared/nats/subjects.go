// Package nats
//
// subject defines the unified messaging taxonomy for the SpaceScale platform.
//
// It acts as the central registry for all NATS Subjects and Queue Groups used
// for communication between the stateless Control Plane (scalecp) and the
// autonomous Edge daemons (scaled).
//
// Architecture: The Decentralized Auction
//
// SpaceScale avoids centralized, database-locked scheduling. Instead, it relies
// on an event-driven Request-Reply pattern. The control plane broadcasts one
// resolved microvm shape, and edge nodes independently calculate their real time
// physical capacity to bid on that workload.
//
// Subjects vs. Queue Groups
//
// To achieve infinite horizontal scaling, this package makes a strict distinction
// between Subjects and Queue Groups:
//
//   - Subjects are the "Radio Frequencies". They dictate WHERE a message goes.
//     For example, all nodes in a region listen to the same auction Subject.
//
//   - Queue Groups are the "Call Centers". They dictate WHO processes a message.
//     Whenever the Control Plane listens to a Subject, it MUST use a Queue Group.
//     This ensures NATS acts as a load-balancer, delivering the message to exactly
//     one Control Plane instance, preventing database write conflicts.
package nats

import "fmt"

// Subjects , The NAT Routing Addresses
const (
	// SubjectNodeBootstrap is used by a new scale daemon to announce its  physical specs to the CP and request a permanent identity.
	SubjectNodeBootstrap = "node.bootstrap"

	// SubjectNodeAlert is fired by the Edge daemon when it detects a critical  hardware threshold, e.g. nvme error
	SubjectNodeAlert = "node.alert"

	// SubjectNodeState is fired by the Edge daemon when its operational status changes e.g. from READY TO CORDONED
	SubjectNodeState = "node.state.changed"

	// subjectNodeAuction is the regional broadcast frequency. CP publishes to this subject to solicit bids for new microvms.
	// %s must be formatted with the target region  e,g, us-east
	subjectNodeAuction = "node.auction.%s.microvm"

	// subjectNodeMicroVMLaunch is the personal inbox for a specific edge daemon. CP publishes launch intent after a node wins an auction.
	// The %s must be formatted with the target node's boot_id.
	subjectNodeMicroVMLaunch = "node.cmd.%s.microvm.launch"
)

// QUEUE GROUPS CONTROL PLANE LOAD BALANCING
// Note: Queue Groups are strictly for Control Plane receivers.We do not use Queue Groups for scale daemons
const (
	// QueueNodeBootstrap ensures only one CP instance writes the new node to Postgres.
	QueueNodeBootstrap = "node-bootstrap"

	// QueueNodeAlert ensures only one CP instance processes a hardware alert.
	QueueNodeAlert = "node-alert"

	// QueueNodeState ensures only one CP instance updates the node's status in Postgres.
	QueueNodeState = "node-state"
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
