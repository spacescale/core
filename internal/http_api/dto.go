package http_api

import (
	"time"

	"github.com/t0gun/paas/internal/domain"
)

type createAppReq struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Port  int    `json:"port"`
}

type appResp struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Image     string           `json:"image"`
	Port      int              `json:"port"`
	Status    domain.AppStatus `json:"status"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

// toAppResp convert domain types which are business logic types to JSON field names. sometimes we can decide to hide
// fields and also create data shapes that might not match api layer. this separation of concerns prevents constant
// breakage
func toAppResp(a domain.App) appResp {
	return appResp{
		ID:        a.ID,
		Name:      a.Name,
		Image:     a.Image,
		Port:      a.Port,
		Status:    a.Status,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

type deploymentResp struct {
	ID        string                  `json:"id"`
	AppID     string                  `json:"appId"`
	Status    domain.DeploymentStatus `json:"status"`
	URL       *string                 `json:"url,omitempty"`
	Error     *string                 `json:"error,omitempty"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
}

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
