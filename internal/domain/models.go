package domain

import (
	"time"

	"github.com/google/uuid"
)

type AppStatus string

const (
	AppStatusCreated  AppStatus = "CREATED"
	AppStatusBuilding AppStatus = "BUILDING"
	AppStatusRunning  AppStatus = "RUNNING"
	AppStatusFiled    AppStatus = "FAILED"
	AppStatusPaused   AppStatus = "PAUSED"
)

type DeploymentStatus string

const (
	DeploymentStatusQueued    DeploymentStatus = "QUEUED"
	DeploymentStatusBuilding  DeploymentStatus = "BUILDING"
	DeploymentStatusDeploying DeploymentStatus = "DEPLOYING"
	DeploymentStatusRunning   DeploymentStatus = "RUNNING"
	DeploymentStatusFailed    DeploymentStatus = "FAILED"
)

type App struct {
	ID        string
	Name      string
	Image     string
	Port      int
	Status    AppStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NewAppParams struct {
	Name  string
	Image string
	Port  int
}

func NewApp(p NewAppParams) (App, error) {
	if err := ValidateAppName(p.Name); err != nil {
		return App{}, err
	}

	if err := ValidateImageRef(p.Image); err != nil {
		return App{}, err
	}
	if err := ValidatePort(p.Port); err != nil {
		return App{}, err
	}

	now := time.Now().UTC()
	return App{
		ID:        uuid.NewString(),
		Name:      p.Name,
		Image:     p.Image,
		Port:      p.Port,
		Status:    AppStatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

type Deployment struct {
	ID        string
	AppID     string
	Status    DeploymentStatus
	Error     *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

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
