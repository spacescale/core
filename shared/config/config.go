// Package config reads startup settings from environment variables.
package config

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

const (
	defaultListenAddr = ":8080"
	workOSCookieName  = "spacescale_session"

	defaultPlacementRegions = "us-east,us-west,eu-central,eu-west,ca-central,ca-east"
	defaultPlacementRegion  = "us-east"
	defaultPlacementGeo     = "CA:ca-central,ca-east,us-east;US:us-east,us-west,ca-central;EU:eu-central,eu-west,us-east"
)

var configValidator = validator.New(validator.WithRequiredStructEnabled())

// Control is the runtime configuration for the control plane.
type Control struct {
	Environment        string `validate:"required,oneof=development production"`
	NATSURL            string `validate:"required,url"`
	DatabaseURL        string `validate:"required"`
	ListenAddr         string `validate:"required"`
	WorkOS             WorkOSConfig
	Placement          PlacementConfig
	EnvEncryptionKeyID string `validate:"required"`
	EnvEncryptionKey   string `validate:"required,base64,min=44,max=44"`

	// TrustedProxies lists source networks allowed to set client-origin
	// headers (CF-Connecting-IP, CF-IPCountry). Requests from any other
	// address have those headers ignored. Empty means no proxy is trusted.
	TrustedProxies []netip.Prefix
}

// PlacementConfig holds Ignite region catalog and auto-placement policy.
type PlacementConfig struct {
	Regions       []string
	DefaultRegion string
	GeoPriority   map[string][]string
}

// Scaled is the runtime configuration for the edge daemon.
type Scaled struct {
	Environment string `validate:"required,oneof=development production"`
	NATSURL     string `validate:"required,url"`
}

// WorkOSConfig holds the WorkOS settings used by the control plane.
type WorkOSConfig struct {
	APIKey               string `validate:"required"`
	ClientID             string `validate:"required"`
	CookiePassword       string `validate:"required,min=32"`
	RedirectURI          string `validate:"required,url"`
	PostLoginRedirectURI string `validate:"required,url"`
	LogoutRedirectURI    string `validate:"required,url"`
	CookieName           string `validate:"required"`
}

type controlEnv struct {
	Environment        string `env:"ENVIRONMENT"`
	NATSURL            string `env:"NATS_URL"`
	DatabaseURL        string `env:"DATABASE_URL"`
	EnvEncryptionKeyID string `env:"API_ENV_ENCRYPTION_KEY_ID"`
	EnvEncryptionKey   string `env:"API_ENV_ENCRYPTION_KEY"`

	WorkOSAPIKey               string `env:"WORKOS_API_KEY"`
	WorkOSClientID             string `env:"WORKOS_CLIENT_ID"`
	WorkOSCookiePassword       string `env:"WORKOS_COOKIE_PASSWORD"`
	WorkOSRedirectURI          string `env:"WORKOS_REDIRECT_URI"`
	WorkOSPostLoginRedirectURI string `env:"WORKOS_POST_LOGIN_REDIRECT_URI"`
	WorkOSLogoutRedirectURI    string `env:"WORKOS_LOGOUT_REDIRECT_URI"`

	PlacementRegions       string `env:"PLACEMENT_REGIONS"`
	PlacementDefaultRegion string `env:"PLACEMENT_DEFAULT_REGION"`
	PlacementGeoPriority   string `env:"PLACEMENT_GEO_PRIORITY"`

	TrustedProxyCIDRs string `env:"TRUSTED_PROXY_CIDRS"`
}

type scaledEnv struct {
	Environment string `env:"ENVIRONMENT"`
	NATSURL     string `env:"NATS_URL"`
}

// LoadControl reads control-plane config from the environment.
func LoadControl() (Control, error) {
	raw, err := env.ParseAs[controlEnv]()
	if err != nil {
		return Control{}, err
	}

	cfg := Control{
		Environment: strings.TrimSpace(raw.Environment),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
		DatabaseURL: strings.TrimSpace(raw.DatabaseURL),
		WorkOS: WorkOSConfig{
			APIKey:               strings.TrimSpace(raw.WorkOSAPIKey),
			ClientID:             strings.TrimSpace(raw.WorkOSClientID),
			CookiePassword:       strings.TrimSpace(raw.WorkOSCookiePassword),
			RedirectURI:          strings.TrimSpace(raw.WorkOSRedirectURI),
			PostLoginRedirectURI: strings.TrimSpace(raw.WorkOSPostLoginRedirectURI),
			LogoutRedirectURI:    strings.TrimSpace(raw.WorkOSLogoutRedirectURI),
			CookieName:           workOSCookieName,
		},
		Placement:          parsePlacementConfig(raw.PlacementRegions, raw.PlacementDefaultRegion, raw.PlacementGeoPriority),
		EnvEncryptionKeyID: strings.TrimSpace(raw.EnvEncryptionKeyID),
		EnvEncryptionKey:   strings.TrimSpace(raw.EnvEncryptionKey),
	}
	trustedProxies, err := parseTrustedProxies(raw.TrustedProxyCIDRs)
	if err != nil {
		return Control{}, err
	}
	cfg.TrustedProxies = trustedProxies
	cfg.ListenAddr = defaultListenAddr
	if err := configValidator.Struct(cfg); err != nil {
		return Control{}, err
	}

	return cfg, nil
}

func parsePlacementConfig(regionsRaw, defaultRaw, geoRaw string) PlacementConfig {
	regionsRaw = strings.TrimSpace(regionsRaw)
	if regionsRaw == "" {
		regionsRaw = defaultPlacementRegions
	}
	defaultRaw = strings.TrimSpace(defaultRaw)
	if defaultRaw == "" {
		defaultRaw = defaultPlacementRegion
	}
	geoRaw = strings.TrimSpace(geoRaw)
	if geoRaw == "" {
		geoRaw = defaultPlacementGeo
	}

	return PlacementConfig{
		Regions:       splitCSV(regionsRaw),
		DefaultRegion: strings.ToLower(defaultRaw),
		GeoPriority:   parseGeoPriority(geoRaw),
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

// parseTrustedProxies parses a CSV of CIDRs or plain IPs into prefixes.
// Plain IPs become single-address prefixes so operators can list either form.
func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	entries := splitCSV(raw)
	if len(entries) == 0 {
		return nil, nil
	}

	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, entry := range entries {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS entry %q: %w", entry, err)
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, nil
}

// parseGeoPriority parses "CA:ca-central,us-east;US:us-east,us-west".
func parseGeoPriority(raw string) map[string][]string {
	out := make(map[string][]string)
	for _, group := range strings.Split(raw, ";") {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		country, regions, ok := strings.Cut(group, ":")
		if !ok {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(country))
		if code == "" {
			continue
		}
		list := splitCSV(regions)
		if len(list) == 0 {
			continue
		}
		out[code] = list
	}
	return out
}

// LoadScaled reads edge-daemon config from the environment.
func LoadScaled() (Scaled, error) {
	raw, err := env.ParseAs[scaledEnv]()
	if err != nil {
		return Scaled{}, err
	}

	cfg := Scaled{
		Environment: strings.TrimSpace(raw.Environment),
		NATSURL:     strings.TrimSpace(raw.NATSURL),
	}
	if err := configValidator.Struct(cfg); err != nil {
		return Scaled{}, err
	}
	return cfg, nil
}
