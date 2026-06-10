package nats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeAuctionSubject(t *testing.T) {
	tests := []struct {
		name   string
		region string
		want   string
	}{
		{name: "single region", region: "us-east", want: "node.auction.us-east.microvm"},
		{name: "region with dash", region: "eu-west-1", want: "node.auction.eu-west-1.microvm"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NodeAuctionSubject(tc.region))
		})
	}
}

func TestNodeMicroVMLaunchSubject(t *testing.T) {
	tests := []struct {
		name   string
		bootID string
		want   string
	}{
		{name: "simple boot id", bootID: "boot-12345", want: "node.cmd.boot-12345.microvm.launch"},
		{name: "boot id with suffix", bootID: "boot-abc-1", want: "node.cmd.boot-abc-1.microvm.launch"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NodeMicroVMLaunchSubject(tc.bootID))
		})
	}
}
