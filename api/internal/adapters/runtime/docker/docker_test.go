// Docker runtime tests for deploy behavior.
package docker_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/t0gun/spacescale/internal/adapters/runtime/docker"
	"github.com/t0gun/spacescale/internal/domain"
)

// TestDockerRuntime_Deploy runs a Docker-backed deploy with explicit port.
func TestDockerRuntime_Deploy(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("set RUN_DOCKER_TESTS=1 to run docker integration tests")
	}

	rt, err := docker.New(
		docker.WithEdge(docker.EdgeConfig{
			BaseDomain: "localtest.me",
			TraefikNet: "traefik",
			Scheme:     "web", // entrypoint name
			EnableTLS:  false,
		}),
	)
	assert.NoError(t, err)

	app, err := domain.NewApp(domain.NewAppParams{
		Name:  "hello",
		Image: "nginx:latest",
		Port:  ptrInt(80),
	})
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	url, err := rt.Deploy(ctx, app)
	assert.NoError(t, err)

	assert.NotNil(t, url)
	assert.Equal(t, "http://hello.localtest.me", *url)
}

// ptrInt returns a pointer to the provided int.
func ptrInt(v int) *int {
	return &v
}

// TestDockerRuntime_Deploy_EmptyImage validates empty image handling.
func TestDockerRuntime_Deploy_EmptyImage(t *testing.T) {
	rt, err := docker.New()
	assert.NoError(t, err)
	app := domain.App{Name: "app", Image: "", Expose: false}
	url, err := rt.Deploy(context.Background(), app)
	assert.Nil(t, url)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker runtime: empty image")
}

// TestDockerRuntime_Deploy_EmptyBaseDomain validates missing base domain.
func TestDockerRuntime_Deploy_EmptyBaseDomain(t *testing.T) {
	rt, err := docker.New(docker.WithEdge(docker.EdgeConfig{
		BaseDomain: "",
		TraefikNet: "traefik",
		Scheme:     "web",
	}))
	assert.NoError(t, err)
	app := domain.App{Name: "app", Image: "nginx:latest", Expose: true}
	url, err := rt.Deploy(context.Background(), app)
	assert.Nil(t, url)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker runtime: empty base domain")
}

// TestDockerRuntime_Deploy_EmptyTraefikNet validates missing traefik network.
func TestDockerRuntime_Deploy_EmptyTraefikNet(t *testing.T) {
	rt, err := docker.New(docker.WithEdge(docker.EdgeConfig{
		BaseDomain: "localtest.me",
		TraefikNet: "",
		Scheme:     "web",
	}))
	assert.NoError(t, err)
	app := domain.App{Name: "app", Image: "nginx:latest", Expose: true}
	url, err := rt.Deploy(context.Background(), app)
	assert.Nil(t, url)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker runtime: empty traefik network")
}

// TestDockerRuntime_Deploy_ImplicitPort resolves port from image metadata.
func TestDockerRuntime_Deploy_ImplicitPort(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("set RUN_DOCKER_TESTS=1 to run docker integration tests")
	}
	rt, err := docker.New(docker.WithEdge(docker.EdgeConfig{
		BaseDomain: "localtest.me",
		TraefikNet: "traefik",
		Scheme:     "web",
		EnableTLS:  false,
	}))
	assert.NoError(t, err)
	app, err := domain.NewApp(domain.NewAppParams{
		Name:  "hello-implicit",
		Image: "nginx:latest",
	})
	assert.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	url, err := rt.Deploy(ctx, app)
	assert.NoError(t, err)
	assert.NotNil(t, url)
	assert.Equal(t, "http://hello-implicit.localtest.me", *url)
}

// TestDockerRuntime_Deploy_NoExpose returns nil URL when not exposed.
func TestDockerRuntime_Deploy_NoExpose(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("set RUN_DOCKER_TESTS=1 to run docker integration tests")
	}
	rt, err := docker.New(docker.WithEdge(docker.EdgeConfig{
		BaseDomain: "localtest.me",
		TraefikNet: "traefik",
		Scheme:     "web",
		EnableTLS:  false,
	}))
	assert.NoError(t, err)
	app := domain.App{
		Name:   "hello-noexpose",
		Image:  "nginx:latest",
		Expose: false,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	url, err := rt.Deploy(ctx, app)
	assert.NoError(t, err)
	assert.Nil(t, url)
}
