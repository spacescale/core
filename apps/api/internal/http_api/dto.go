// Http api request and response shapes
// Types map domain models to json payloads
// Optional port expose and env fields are supported
// Deployment responses include url and error fields
// These shapes keep api payloads consistent

package http_api

import (
	"time"

	"github.com/t0gun/spacescale/internal/domain"
)

// createAppReq is the request body for creating an app
type createAppReq struct {
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Port   *int              `json:"port,omitempty"`
	Expose *bool             `json:"expose,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

// appResp is the API response shape for an app
type appResp struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Port      *int              `json:"port,omitempty"`
	Expose    bool              `json:"expose"`
	Env       map[string]string `json:"env,omitempty"`
	Status    domain.AppStatus  `json:"status"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// toAppResp maps a domain app to the API response shape.
func toAppResp(a domain.App) appResp {
	return appResp{
		ID:        a.ID,
		Name:      a.Name,
		Image:     a.Image,
		Port:      a.Port,
		Expose:    a.Expose,
		Env:       a.Env,
		Status:    a.Status,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

// deploymentResp is the API response shape for a deployment
type deploymentResp struct {
	ID        string                  `json:"id"`
	AppID     string                  `json:"appId"`
	Status    domain.DeploymentStatus `json:"status"`
	URL       *string                 `json:"url,omitempty"`
	Error     *string                 `json:"error,omitempty"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
}

// toDeploymentResp maps a domain deployment to the API response shape.
func toDeploymentResp(d domain.Deployment) deploymentResp {
	return deploymentResp{
		ID:        d.ID,
		AppID:     d.AppID,
		Status:    d.Status,
		URL:       d.URL,
		Error:     d.Error,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}
