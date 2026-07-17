package placement

import (
	"errors"
	"fmt"
)

// ErrUnknownRegion reports that an explicit region is not in the catalog.
var ErrUnknownRegion = errors.New("unknown region")

// UnknownRegion returns a typed unknown-region error with the requested value.
func UnknownRegion(region string) error {
	return fmt.Errorf("%w: %s", ErrUnknownRegion, normalizeRegion(region))
}
