// Copyright (c) 2026 SpaceScale Systems Inc. All rights reserved.

// Package catalog defines the product plans sold by the control plane.
//
// This package is the only place that knows product plan ids, display metadata,
// and the resolved microvm shape behind each plan. The edge never
// sees these plan ids. It only receives the resolved microvm shape over the
// shared transport.
package catalog

import (
	"strings"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
)

// Plan describes one sellable product plan.
//
// Shape is the resolved microvm shape that will be sent to the edge.
type Plan struct {
	ID          string
	DisplayName string
	Shape       pb.MicroVMShape
	Hints       []string
}

// Plans is the static product catalog used by the control plane.
var Plans = map[string]Plan{
	"web-hobby": {
		ID:          "web-hobby",
		DisplayName: "Hobby Web",
		Shape: pb.MicroVMShape{
			Vcpu:     1,
			RamMb:    1024,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 0,
		},
		Hints: []string{"balanced compute"},
	},
	"web-starter": {
		ID:          "web-starter",
		DisplayName: "Starter Web",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    2048,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 0,
		},
		Hints: []string{"balanced compute"},
	},
	"web-growth": {
		ID:          "web-growth",
		DisplayName: "Growth Web",
		Shape: pb.MicroVMShape{
			Vcpu:     4,
			RamMb:    4096,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 0,
		},
		Hints: []string{"balanced compute"},
	},
	"web-pro": {
		ID:          "web-pro",
		DisplayName: "Pro Web",
		Shape: pb.MicroVMShape{
			Vcpu:     4,
			RamMb:    8192,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"dedicated compute"},
	},
	"web-scale": {
		ID:          "web-scale",
		DisplayName: "Scale Web",
		Shape: pb.MicroVMShape{
			Vcpu:     8,
			RamMb:    16384,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"dedicated compute"},
	},
	"web-ultra": {
		ID:          "web-ultra",
		DisplayName: "Ultra Web",
		Shape: pb.MicroVMShape{
			Vcpu:     16,
			RamMb:    32768,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"dedicated compute"},
	},
	"db-dev": {
		ID:          "db-dev",
		DisplayName: "Dev Database",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    2048,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 10240,
		},
		Hints: []string{"disk io optimized", "memory heavy"},
	},
	"db-prod": {
		ID:          "db-prod",
		DisplayName: "Production Database",
		Shape: pb.MicroVMShape{
			Vcpu:     4,
			RamMb:    16384,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 51200,
		},
		Hints: []string{"disk io optimized", "memory heavy"},
	},
	"db-scale": {
		ID:          "db-scale",
		DisplayName: "Scale Database",
		Shape: pb.MicroVMShape{
			Vcpu:     8,
			RamMb:    32768,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 102400,
		},
		Hints: []string{"disk io optimized", "memory heavy"},
	},
	"db-enterprise": {
		ID:          "db-enterprise",
		DisplayName: "Enterprise Database",
		Shape: pb.MicroVMShape{
			Vcpu:     16,
			RamMb:    65536,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 204800,
		},
		Hints: []string{"disk io optimized", "memory heavy"},
	},
	"db-apex": {
		ID:          "db-apex",
		DisplayName: "Apex Database",
		Shape: pb.MicroVMShape{
			Vcpu:     24,
			RamMb:    131072,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 512000,
		},
		Hints: []string{"disk io optimized", "memory heavy"},
	},
	"cache-micro": {
		ID:          "cache-micro",
		DisplayName: "Micro Cache",
		Shape: pb.MicroVMShape{
			Vcpu:     1,
			RamMb:    2048,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 0,
		},
		Hints: []string{"ram heavy", "single threaded"},
	},
	"cache-standard": {
		ID:          "cache-standard",
		DisplayName: "Standard Cache",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    8192,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"ram heavy", "single threaded"},
	},
	"cache-large": {
		ID:          "cache-large",
		DisplayName: "Large Cache",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    16384,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"ram heavy", "single threaded"},
	},
	"cache-massive": {
		ID:          "cache-massive",
		DisplayName: "Massive Cache",
		Shape: pb.MicroVMShape{
			Vcpu:     4,
			RamMb:    65536,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"ram heavy", "single threaded"},
	},
	"broker-standard": {
		ID:          "broker-standard",
		DisplayName: "Standard Broker",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    4096,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 10240,
		},
		Hints: []string{"network throughput optimized", "high concurrent connections"},
	},
	"broker-core": {
		ID:          "broker-core",
		DisplayName: "Core Broker",
		Shape: pb.MicroVMShape{
			Vcpu:     4,
			RamMb:    16384,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 51200,
		},
		Hints: []string{"network throughput optimized", "high concurrent connections"},
	},
	"broker-global": {
		ID:          "broker-global",
		DisplayName: "Global Broker",
		Shape: pb.MicroVMShape{
			Vcpu:     8,
			RamMb:    32768,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 102400,
		},
		Hints: []string{"network throughput optimized", "high concurrent connections"},
	},
	"worker-small": {
		ID:          "worker-small",
		DisplayName: "Small Worker",
		Shape: pb.MicroVMShape{
			Vcpu:     2,
			RamMb:    1024,
			CpuMode:  pb.CpuMode_CPU_MODE_SHARED,
			VolumeMb: 0,
		},
		Hints: []string{"compute heavy", "headless"},
	},
	"worker-heavy": {
		ID:          "worker-heavy",
		DisplayName: "Heavy Worker",
		Shape: pb.MicroVMShape{
			Vcpu:     8,
			RamMb:    8192,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"compute heavy", "headless"},
	},
	"worker-max": {
		ID:          "worker-max",
		DisplayName: "Max Worker",
		Shape: pb.MicroVMShape{
			Vcpu:     16,
			RamMb:    16384,
			CpuMode:  pb.CpuMode_CPU_MODE_PINNED,
			VolumeMb: 0,
		},
		Hints: []string{"compute heavy", "headless"},
	},
}

// LookupPlan resolves one plan id from the static catalog.
func LookupPlan(raw string) (Plan, bool) {
	plan, ok := Plans[strings.ToLower(strings.TrimSpace(raw))]
	return plan, ok
}

//The Unit Economics (Internal Logic)
//
//Shared vCPU (4:1 Density): $15.00 / month
//
//pCore (Dedicated Physical Core - 1:1): $75.00 / month
//
//RAM (ECC Memory): $8.00 / GB / month
//
//Storage (NVMe Volume): $0.20 / GB / month
// The Term PCore Will be shown at the front end
