package fabric

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPlaceFallsBackAcrossRegions(t *testing.T) {
	dispatcher := &Dispatcher{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	calls := 0
	original := auctionFn
	auctionFn = func(_ *Dispatcher, req Request) (Winner, error) {
		calls++
		switch req.Region {
		case "ca-central":
			return Winner{}, ErrNoAuctionBids
		case "us-east":
			return Winner{NodeID: uuid.NewString(), BootID: "boot-1", Region: "us-east"}, nil
		default:
			return Winner{}, errors.New("unexpected region")
		}
	}
	t.Cleanup(func() { auctionFn = original })

	winner, err := dispatcher.place(Request{
		Region:  "ca-central",
		Regions: []string{"ca-central", "us-east"},
	})
	require.NoError(t, err)
	require.Equal(t, "us-east", winner.Region)
	require.Equal(t, 2, calls)
}

func TestPlaceStopsOnNonCapacityError(t *testing.T) {
	dispatcher := &Dispatcher{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	calls := 0
	original := auctionFn
	auctionFn = func(_ *Dispatcher, _ Request) (Winner, error) {
		calls++
		return Winner{}, errors.New("nats down")
	}
	t.Cleanup(func() { auctionFn = original })

	_, err := dispatcher.place(Request{
		Regions: []string{"ca-central", "us-east"},
	})
	require.ErrorContains(t, err, "nats down")
	require.Equal(t, 1, calls)
}
