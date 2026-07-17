// Package placement owns Ignite region selection: known-region validation,
// automatic candidate ordering, and geo-bucket fallback lists.
package placement

import (
	"errors"
	"fmt"
	"strings"
)

// Catalog is the configured set of known placement regions and geo priorities.
type Catalog struct {
	regions       map[string]struct{}
	ordered       []string
	defaultRegion string
	geoPriority   map[string][]string
}

// Config is the durable placement policy loaded from control-plane settings.
type Config struct {
	Regions       []string
	DefaultRegion string
	GeoPriority   map[string][]string
}

// Plan is the resolved placement strategy for one deploy request.
type Plan struct {
	Requested  string
	Automatic  bool
	Candidates []string
}

// NewCatalog validates and freezes a placement configuration.
func NewCatalog(cfg Config) (*Catalog, error) {
	regions := make([]string, 0, len(cfg.Regions))
	seen := make(map[string]struct{}, len(cfg.Regions))
	for _, raw := range cfg.Regions {
		region := normalizeRegion(raw)
		if region == "" {
			continue
		}
		if err := validateRegionToken(region); err != nil {
			return nil, err
		}
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		regions = append(regions, region)
	}
	if len(regions) == 0 {
		return nil, errors.New("placement catalog requires at least one known region")
	}

	defaultRegion := normalizeRegion(cfg.DefaultRegion)
	if defaultRegion == "" {
		defaultRegion = regions[0]
	}
	if _, ok := seen[defaultRegion]; !ok {
		return nil, fmt.Errorf("default region %q is not in the known region catalog", defaultRegion)
	}

	geoPriority := make(map[string][]string, len(cfg.GeoPriority))
	for country, list := range cfg.GeoPriority {
		code := strings.ToUpper(strings.TrimSpace(country))
		if code == "" {
			continue
		}
		filtered := make([]string, 0, len(list))
		localSeen := make(map[string]struct{}, len(list))
		for _, raw := range list {
			region := normalizeRegion(raw)
			if region == "" {
				continue
			}
			if _, ok := seen[region]; !ok {
				return nil, fmt.Errorf("geo priority for %s references unknown region %q", code, region)
			}
			if _, ok := localSeen[region]; ok {
				continue
			}
			localSeen[region] = struct{}{}
			filtered = append(filtered, region)
		}
		if len(filtered) > 0 {
			geoPriority[code] = filtered
		}
	}

	return &Catalog{
		regions:       seen,
		ordered:       regions,
		defaultRegion: defaultRegion,
		geoPriority:   geoPriority,
	}, nil
}

// DefaultRegion returns the configured automatic-placement default.
func (c *Catalog) DefaultRegion() string {
	return c.defaultRegion
}

// Known reports whether region is in the catalog.
func (c *Catalog) Known(region string) bool {
	_, ok := c.regions[normalizeRegion(region)]
	return ok
}

// Resolve builds the auction candidate list for one request.
//
// Explicit region requests are validated and never fall back.
// Omitted region requests use geo priority when a country is available,
// otherwise the configured default region followed by remaining known regions.
func (c *Catalog) Resolve(requestedRegion, countryCode string) (Plan, error) {
	requested := normalizeRegion(requestedRegion)
	if requested != "" {
		if !c.Known(requested) {
			return Plan{}, UnknownRegion(requested)
		}
		return Plan{
			Requested:  requested,
			Automatic:  false,
			Candidates: []string{requested},
		}, nil
	}

	candidates := c.automaticCandidates(countryCode)
	return Plan{
		Requested:  "",
		Automatic:  true,
		Candidates: candidates,
	}, nil
}

func (c *Catalog) automaticCandidates(countryCode string) []string {
	code := strings.ToUpper(strings.TrimSpace(countryCode))
	if list, ok := c.geoPriority[code]; ok && len(list) > 0 {
		return appendRemaining(list, c.ordered)
	}

	return appendRemaining([]string{c.defaultRegion}, c.ordered)
}

func appendRemaining(primary, all []string) []string {
	out := make([]string, 0, len(all))
	seen := make(map[string]struct{}, len(all))
	for _, region := range primary {
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		out = append(out, region)
	}
	for _, region := range all {
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		out = append(out, region)
	}
	return out
}

func normalizeRegion(region string) string {
	return strings.ToLower(strings.TrimSpace(region))
}

// validateRegionToken rejects values that would break NATS subject formatting.
func validateRegionToken(region string) error {
	if region == "" || len(region) > 32 {
		return fmt.Errorf("invalid region %q", region)
	}
	for _, char := range region {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= '0' && char <= '9':
		case char == '-' || char == '_':
		default:
			return fmt.Errorf("invalid region %q", region)
		}
	}
	return nil
}
