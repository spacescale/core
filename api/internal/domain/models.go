// Domain models for apps and deployments in the service
// Status values describe lifecycle states for apps and deployments
// New app creation applies validation and default exposure
// Timestamps are stored in utc for consistent records
// Env values are stored as simple key value maps

package domain

import (
	"time"

	"github.com/google/uuid"
)

// AppStatus represents the lifecycle state of an app
type AppStatus string

const (
	AppStatusCreated  AppStatus = "CREATED"
	AppStatusBuilding AppStatus = "BUILDING"
	AppStatusRunning  AppStatus = "RUNNING"
	AppStatusFailed   AppStatus = "FAILED"
	AppStatusPaused   AppStatus = "PAUSED"
)

// DeploymentStatus represents the lifecycle state of a deployment
type DeploymentStatus string

const (
	DeploymentStatusQueued    DeploymentStatus = "QUEUED"
	DeploymentStatusBuilding  DeploymentStatus = "BUILDING"
	DeploymentStatusDeploying DeploymentStatus = "DEPLOYING"
	DeploymentStatusRunning   DeploymentStatus = "RUNNING"
	DeploymentStatusFailed    DeploymentStatus = "FAILED"
)

// App is the core application model stored by the platform
type App struct {
	ID        string
	Name      string
	Image     string
	Port      *int
	Expose    bool
	Env       map[string]string
	Status    AppStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewAppParams holds the input used to construct an App
type NewAppParams struct {
	Name   string
	Image  string
	Port   *int
	Expose *bool // nil defaults to true
	Env    map[string]string
}

// NewApp builds a validated App from input parameters.
func NewApp(p NewAppParams) (App, error) {
	if err := ValidateAppName(p.Name); err != nil {
		return App{}, err
	}

	if err := ValidateImageRef(p.Image); err != nil {
		return App{}, err
	}

	exposeVal := true
	if p.Expose != nil {
		exposeVal = *p.Expose
	}
	if err := ValidatePort(p.Port); err != nil {
		return App{}, err
	}

	var envCopy map[string]string
	if p.Env != nil {
		envCopy = make(map[string]string, len(p.Env))
		for k, v := range p.Env {
			envCopy[k] = v
		}
	}

	now := time.Now().UTC()
	return App{
		ID:        uuid.NewString(),
		Name:      p.Name,
		Image:     p.Image,
		Port:      p.Port,
		Expose:    exposeVal,
		Env:       envCopy,
		Status:    AppStatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Deployment tracks a single deployment attempt for an app
type Deployment struct {
	ID        string
	AppID     string
	Status    DeploymentStatus
	URL       *string
	Error     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewDeployment builds a queued Deployment for an app.
func NewDeployment(appID string) Deployment {
	now := time.Now().UTC()
	return Deployment{
		ID:        uuid.NewString(),
		AppID:     appID,
		Status:    DeploymentStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
