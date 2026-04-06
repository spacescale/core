// Package placement implements the decentralized scheduling engine for the edge node.
//
// This file defines the static economic and hardware taxation policies for the
// node. It calculates exactly how much physical RAM and how many
// PHYSICAL cores are held back for the host operating system, ensuring customer
// workloads cannot starve the host hypervisor or the scaled daemon itself.
//
// NOTE: SpaceScale operates strictly on a Physical Core Truth model.
// SMT (Hyperthreading) is considered a security vulnerability and is disabled.
// This file does not enforce runtime pinning, cgroups, or cpusets. It only
// answers the capacity policy question of how much of the host may be sold to
// customer workloads once the operating system tax is applied.
package placement

const (
	// sharedVCPUOvercommitRatio defines how many shared guest vCPUs may be sold
	// per remaining physical host core. This ratio applies only to the shared
	// pool after host reserved cores and dedicated pinned workloads have already
	// been removed from the node capacity.
	//
	// Since SpaceScale operates on a Physical Core Truth model with SMT disabled,
	// a ratio of 4 ensures that even under maximum contention, each virtual CPU
	// is guaranteed at least 25% of a physical core's execution time. This balances
	// high-density multi-tenancy with deterministic, "snappy" performance.
	sharedVCPUOvercommitRatio uint32 = 4
	// Nodes up to and including this size reserve one host core.
	singleHostReserveCoreCeiling uint32 = 64
	// Smaller nodes keep one core for the host. Very large nodes keep two.
	baseHostReservedCores  uint32 = 1
	largeHostReservedCores uint32 = 2
)

const (
	// Node RAM thresholds in Megabytes. These determine the progressive host
	// memory tax applied before any customer workload capacity is exposed.
	node32GBMB uint64 = 32768
	node64GBMB uint64 = 65536
	// hostTax constants define the exact amount of RAM in Megabytes that is
	// withheld from the customer workload pool. This pays for the Linux kernel,
	// system services, the scaled daemon, network buffers, and microVM overhead.
	hostTax32GBMB     uint64 = 3686
	hostTax64GBMB     uint64 = 5324
	hostTaxOver64GBMB uint64 = 8601
)

// hostReservedCores returns how many physical cores must always remain with the
// host operating system and control processes.
//
// The current policy is:
//
// 1. Nodes with up to 64 physical cores reserve 1 host core.
// 2. Nodes above 64 physical cores reserve 2 host cores.
//
// This policy assumes routing and other heavier data plane work is offloaded
// elsewhere, so the node host only needs a small protected CPU slice for
// scaled, systemd, tap and bridge work, and basic operating system activity.
func hostReservedCores(total uint32) uint32 {
	switch {
	case total == 0:
		return 0
	case total > singleHostReserveCoreCeiling:
		return largeHostReservedCores
	default:
		return baseHostReservedCores
	}
}

// sellableCores calculates the final pool of physical host cores available for
// customer workloads after the host reserve is removed.
//
// Callers must pass the true physical core count discovered by host preflight.
// This function must not be fed logical thread counts.
func sellableCores(total uint32) uint32 {
	reserved := hostReservedCores(total)
	if total <= reserved {
		return 0
	}
	return total - reserved
}

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
