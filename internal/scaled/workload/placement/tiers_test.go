package placement

import (
	"errors"
	"testing"

	pb "github.com/spacescale/core/internal/shared/pb/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateTier(t *testing.T) {
	tests := []struct {
		name    string
		tier    pb.Tier
		want    HardwareSpec
		wantErr error
	}{
		{
			name: "starter",
			tier: pb.Tier_TIER_STARTER,
			want: HardwareSpec{VCPU: 2, RAM: 4096, IsPinned: false},
		},
		{
			name: "growth",
			tier: pb.Tier_TIER_GROWTH,
			want: HardwareSpec{VCPU: 4, RAM: 8192, IsPinned: false},
		},
		{
			name: "scale",
			tier: pb.Tier_TIER_SCALE,
			want: HardwareSpec{VCPU: 8, RAM: 16384, IsPinned: true},
		},
		{
			name:    "unknown",
			tier:    pb.Tier_TIER_UNSPECIFIED,
			wantErr: ErrUnknownTier,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TranslateTier(tc.tier)
			if tc.wantErr != nil {
				require.True(t, errors.Is(err, tc.wantErr))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
