// Docker runtime for deploying apps locally.
// It pulls images, creates containers, and configures routing labels.
// Ports come from app input or image metadata.
// URLs are returned only when apps are exposed.

package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/t0gun/spacescale/internal/domain"
)

// Runtime deploys apps using the local Docker engine.
type Runtime struct {
	cli           *client.Client
	advertiseHost string
	namePrefix    string
	timeout       time.Duration

	// edge routing config
	edge EdgeConfig
}

// EdgeConfig configures Traefik routing for exposed apps.
type EdgeConfig struct {
	BaseDomain   string // root domain for app hostnames (app.example.com)
	TraefikNet   string // Docker network for Traefik
	Scheme       string // Traefik entrypoint (web/websecure)
	EnableTLS    bool   // enable TLS termination
	CertResolver string // certificate resolver name
}

const errPortRequiredMsg = "port required or image must expose exactly one port"

// Option configures Runtime construction.
type Option func(*Runtime)

// WithAdvertiseHost sets the host used for advertised URLs.
func WithAdvertiseHost(host string) Option { return func(r *Runtime) { r.advertiseHost = host } }

// WithNamePrefix sets the container name prefix.
func WithNamePrefix(prefix string) Option { return func(r *Runtime) { r.namePrefix = prefix } }

// WithTimeout sets the deploy timeout.
func WithTimeout(d time.Duration) Option { return func(r *Runtime) { r.timeout = d } }

// WithEdge overrides edge routing settings.
func WithEdge(cfg EdgeConfig) Option { return func(r *Runtime) { r.edge = cfg } }

// New creates a Runtime with defaults and Docker client.
func New(opts ...Option) (*Runtime, error) {
	cli, err := client.New(
		client.FromEnv,
		client.WithAPIVersionFromEnv(),
	)
	if err != nil {
		return nil, err
	}
	// default runtime before options
	r := &Runtime{
		cli:           cli,
		advertiseHost: "127.0.0.1",
		namePrefix:    "sample-app-",
		timeout:       2 * time.Minute,
		edge: EdgeConfig{
			BaseDomain: "localtest.me",
			TraefikNet: "traefik",
			Scheme:     "web",
			EnableTLS:  false,
		},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// Deploy pulls the image, creates the container, and returns a URL when exposed.
func (r *Runtime) Deploy(ctx context.Context, app domain.App) (*string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	// validate input
	if strings.TrimSpace(app.Image) == "" {
		return nil, fmt.Errorf("docker runtime: empty image")
	}
	if app.Expose {
		if strings.TrimSpace(r.edge.BaseDomain) == "" {
			return nil, fmt.Errorf("docker runtime: empty base domain")
		}
		if strings.TrimSpace(r.edge.TraefikNet) == "" {
			return nil, fmt.Errorf("docker runtime: empty traefik network")
		}
		if strings.TrimSpace(r.edge.Scheme) == "" {
			// default Traefik entrypoint name
			r.edge.Scheme = "web"
		}
	}

	// pull image
	if err := r.pull(ctx, app.Image); err != nil {
		return nil, fmt.Errorf("docker runtime: pull: %w", err)
	}

	port, err := r.resolvePort(ctx, app)
	if err != nil {
		return nil, err
	}

	// replace existing container
	name := r.namePrefix + app.Name
	_ = r.removeIfExists(ctx, name)

	// base labels
	lbls := map[string]string{
		"spacescale.app": app.Name,
	}
	if app.Expose {
		if port == nil {
			return nil, fmt.Errorf(errPortRequiredMsg)
		}
		for k, v := range labelsForApp(app, *port, r.edge) {
			lbls[k] = v
		}
	}

	cfg := &container.Config{
		Image:  app.Image,
		Labels: lbls,
		Env:    envToList(app.Env),
	}
	if port != nil {
		// expose internal port for routing
		cPort, err := network.ParsePort(fmt.Sprintf("%d/tcp", *port))
		if err != nil {
			return nil, fmt.Errorf("docker runtime: parse port: %w", err)
		}
		cfg.ExposedPorts = network.PortSet{cPort: struct{}{}}
	}

	hostcfg := &container.HostConfig{
		PublishAllPorts: false,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}
	if app.Expose {
		// attach to Traefik network
		hostcfg.NetworkMode = container.NetworkMode(r.edge.TraefikNet)
	}

	created, err := r.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     cfg,
		HostConfig: hostcfg,
		Name:       name,
	})
	if err != nil {
		return nil, fmt.Errorf("docker runtime: create: %w", err)
	}
	if _, err := r.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("docker runtime: start container: %w", err)
	}

	// return stable URL
	if !app.Expose {
		return nil, nil
	}
	scheme := "http"
	if r.edge.EnableTLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s.%s", scheme, app.Name, r.edge.BaseDomain)
	return &url, nil
}

// pull pulls an image and drains the response stream.
func (r *Runtime) pull(ctx context.Context, ref string) error {
	rc, err := r.cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)
	return nil
}

// removeIfExists removes a container and ignores not found errors.
func (r *Runtime) removeIfExists(ctx context.Context, name string) error {
	_, err := r.cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true})
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such container") || strings.Contains(msg, "not found") {
		return nil
	}
	return err
}

// resolvePort chooses the port from the app or image when exposed.
func (r *Runtime) resolvePort(ctx context.Context, app domain.App) (*int, error) {
	if app.Port != nil {
		if *app.Port < 1 || *app.Port > 65535 {
			return nil, fmt.Errorf("docker runtime: invalid port %d", *app.Port)
		}
		return app.Port, nil
	}
	if !app.Expose {
		return nil, nil
	}
	port, err := r.portFromImage(ctx, app.Image)
	if err != nil {
		return nil, err
	}
	return &port, nil
}

// portFromImage inspects the image and returns the sole exposed port.
func (r *Runtime) portFromImage(ctx context.Context, ref string) (int, error) {
	inspect, err := r.cli.ImageInspect(ctx, ref)
	if err != nil {
		return 0, fmt.Errorf("docker runtime: inspect image: %w", err)
	}
	exposed := exposedPortsFromInspect(inspect.InspectResponse)
	if len(exposed) != 1 {
		return 0, fmt.Errorf(errPortRequiredMsg)
	}
	port, err := parseExposedPort(exposed[0])
	if err != nil {
		return 0, fmt.Errorf("docker runtime: parse exposed port: %w", err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("docker runtime: invalid port %d", port)
	}
	return port, nil
}

// exposedPortsFromInspect returns sorted exposed ports from image inspect.
func exposedPortsFromInspect(inspect image.InspectResponse) []string {
	if inspect.Config == nil || len(inspect.Config.ExposedPorts) == 0 {
		return nil
	}
	ports := make([]string, 0, len(inspect.Config.ExposedPorts))
	for port := range inspect.Config.ExposedPorts {
		ports = append(ports, port)
	}
	sort.Strings(ports)
	return ports
}

// parseExposedPort parses a port number from an exposed port spec.
func parseExposedPort(spec string) (int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, fmt.Errorf("empty exposed port")
	}
	if idx := strings.Index(spec, "/"); idx != -1 {
		spec = spec[:idx]
	}
	return strconv.Atoi(spec)
}

// envToList converts an env map to sorted KEY=VALUE pairs.
func envToList(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(env))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

// hostPort returns the mapped host port for a container port.
func (r *Runtime) hostPort(ctx context.Context, containerID string, cPort network.Port) (string, error) {
	ins, err := r.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	if ins.Container.NetworkSettings == nil {
		return "", fmt.Errorf("missing network settings")
	}

	b := ins.Container.NetworkSettings.Ports[cPort]
	if len(b) == 0 || strings.TrimSpace(b[0].HostPort) == "" {
		return "", fmt.Errorf("no host port mapped for %s", cPort)
	}
	return b[0].HostPort, nil
}

// labelsForApp builds Traefik v2 labels for one app container.
// It wires host routing, entrypoint, service port, and optional TLS.
// It uses host "<app>.<base-domain>", router "app-<app>", service "svc-<app>".
// It expects port to be the internal container port.
// CertResolver is set only when TLS is enabled.
func labelsForApp(app domain.App, port int, cfg EdgeConfig) map[string]string {
	// build hostname
	host := fmt.Sprintf("%s.%s", app.Name, cfg.BaseDomain)

	// router and service names
	router := "app-" + app.Name
	svc := "svc-" + app.Name

	labels := map[string]string{
		// Traefik v2 labels
		// enable Traefik
		"traefik.enable": "true",

		// Traefik network
		"traefik.docker.network": cfg.TraefikNet,

		// router host rule
		fmt.Sprintf("traefik.http.routers.%s.rule", router): fmt.Sprintf("Host(`%s`)", host),

		// router entrypoint
		fmt.Sprintf("traefik.http.routers.%s.entrypoints", router): cfg.Scheme,

		// service port
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", svc): fmt.Sprintf("%d", port),
	}

	// optional TLS
	if cfg.EnableTLS {
		// enable TLS
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", router)] = "true"

		// set cert resolver
		if cfg.CertResolver != "" {
			labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", router)] = cfg.CertResolver
		}
	}

	return labels
}
