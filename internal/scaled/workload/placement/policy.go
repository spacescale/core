// Package placement implements the decentralized scheduling engine for the edge node.
//
// This file defines the static economic and hardware taxation policies for the
// bare-metal node. It calculates exactly how much physical RAM and how many
// CPU threads are held back for the host operating system, ensuring customer
// workloads cannot starve the host hypervisor or the scaled daemon itself.
package placement

// hostReservedThreads guarantees that a small number of physical threads are
// permanently held back from the customer workload pool. This ensures the
// host operating system, the scaled daemon itself, and essential system
// processes (like network bridges and ssh) always have dedicated CPU time,
// preventing a 100% tenant CPU spike from causing the host to go offline.
const hostReservedThreads uint32 = 2

// sharedVCPUOvercommitRatio dictates the economic and performance model for shared
// compute tiers. It represents how many virtual CPUs can be sold per physical hardware
// thread. A ratio of 3 means 3 virtual CPUs map to 1 physical thread.
//
// This overcommit relies on the statistical probability that most shared workloads
// are idle most of the time allowing the Linux kernel Completely Fair Scheduler
// to rapidly time slice the physical CPU between active workloads. If too many
// virtual CPUs become active simultaneously it causes CPU steal and thrashing.
// A ratio of 3 to 6 is the industry standard sweet spot for general purpose web traffic.
const sharedVCPUOvercommitRatio uint32 = 3

const (
	// Node boundary constants in Megabytes. These are used to determine the
	// progressive taxation rate for the host OS based on the server's physical RAM.
	node32GBMB uint64 = 32768
	node64GBMB uint64 = 65536

	// hostTax constants define the exact amount of RAM (in MB) that is permanently
	// withheld from the customer workload pool. This "tax" pays for the Linux kernel,
	// systemd, the scaled Go daemon, network bridge buffers, and Firecracker overhead.
	hostTax32GBMB     uint64 = 3686 // ~3.6GB reserved for host on 32GB nodes
	hostTax64GBMB     uint64 = 5324 // ~5.3GB reserved for host on 64GB nodes
	hostTaxOver64GBMB uint64 = 8601 // ~8.6GB reserved for host on 128GB+ nodes
)

// hostTaxRAMMB determines the exact amount of RAM in Megabytes that must be
// withheld from the workload pool based on the physical size of the server.
// Larger servers run more microVMs and require a larger networking and
// file descriptor buffer in the kernel.
func hostTaxRAMMB(total uint64) uint64 {
	switch {
	case total <= node32GBMB:
		return hostTax32GBMB
	case total <= node64GBMB:
		return hostTax64GBMB
	default:
		return hostTaxOver64GBMB
	}
}

// sellableRAMMB calculates the final pool of RAM available for customer workloads
// by subtracting the host tax from the total physical RAM.
func sellableRAMMB(total uint64) uint64 {
	tax := hostTaxRAMMB(total)
	if tax >= total {
		return 0
	}
	return total - tax
}

// sellableThreads calculates the final pool of physical CPU threads available
// for customer workloads by withholding the necessary host operating system threads.
func sellableThreads(total uint32) uint32 {
	if total <= hostReservedThreads {
		return 0
	}
	return total - hostReservedThreads
}
