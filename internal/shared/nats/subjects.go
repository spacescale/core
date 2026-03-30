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
// on an event-driven Request-Reply pattern. The Control Plane broadcasts intent
// (e.g., "Need a machine for the Growth tier"), and Edge nodes independently
// calculate their real-time physical capacity to bid on the workload.
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

// Subjects , The NAT Routing Addresses
const (
	// SubjectNodeBootstrap is used by a new scale daemon to announce its  physical specs to the CP and request a permanent identity.
	SubjectNodeBootstrap = "node.bootstrap"

	// SubjectNodeAlert is fired by the Edge daemon when it detects a critical  hardware threshold, e.g. nvme error
	SubjectNodeAlert = "node.alert"

	// SubjectNodeState is fired by the Edge daemon when its operational status changes e.g. from READY TO CORDONED
	SubjectNodeState = "node.state.changed"

	// SubjectNodeAuction is the regional broadcast frequency. CP publishes to this subject to solicit bids for new MicroVM
	// %s must be formatted with the target region  e,g, us-east
	SubjectNodeAuction = "node.auction.%s.machine"

	// SubjectNodeMachineLaunch is the personal inbox for a specific Edge daemon. CP publishes container image and env after a node wins an auction
	// The %s must be formatted with the target node's boot_id.
	SubjectNodeMachineLaunch = "node.cmd.%s.machine.launch"
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
