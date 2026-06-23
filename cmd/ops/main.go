// Command ops is the internal SpaceScale infrastructure management CLI.
//
// ops provides operational tooling for managing the SpaceScale platform itself.
// It is not customer-facing. It is used by SpaceScale engineers to provision
// regions, manage bare metal hosts, inspect platform state, and control
// infrastructure lifecycle.
//
// Capabilities (planned):
//   - Region management: create, inspect, and decommission regional cells
//   - Node management: list nodes, check health, drain, and reboot hosts
//   - NATS inspection: view connected leaves, message rates, consumer lag
//   - Monitoring queries: query VictoriaMetrics and VictoriaLogs from the terminal
//   - Alert management: list, acknowledge, and silence active alerts
//   - Builder control: trigger artifact builds, inspect cache, purge old artifacts
//   - Database operations: check replication lag, trigger failover, run migrations
//   - Golden image management: trigger kernel and rootfs builds, verify host images
//
// ops connects to the same NATS fabric, Postgres, and monitoring backends as
// controlp and scaled. It shares packages from the core module to avoid
// duplicating wire formats, configuration parsing, and connection logic.
package main

import "fmt"

func main() {
	fmt.Println("spacescale-ops is not yet implemented")
}
