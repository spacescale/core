package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spacescale/core/control/fabric"
	"github.com/spacescale/core/control/placement"
	"github.com/spacescale/core/control/tenant"
)

const createWorkloadDispatchTimeout = 20 * time.Second

type createWorkloadEnvVarRequest struct {
	Key      string `json:"key" validate:"required,notblank,envkey"`
	Value    string `json:"value" validate:"max=8192"`
	IsSecret bool   `json:"isSecret"`
}

type createWorkloadComputeRequest struct {
	VCPU      uint32 `json:"vcpu" validate:"gt=0,lte=2147483647"`
	MemoryMB  uint64 `json:"memoryMb" validate:"gt=0,lte=9223372036854775807"`
	Dedicated bool   `json:"dedicated"`
}

type createWorkloadRequest struct {
	Name                 string                        `json:"name" validate:"omitempty,notblank,max=63"`
	ImageRef             string                        `json:"imageRef" validate:"required,notblank,max=1024,excludesall= \t\r\n"`
	Compute              createWorkloadComputeRequest  `json:"compute" validate:"required"`
	PrimaryRegion        string                        `json:"primaryRegion" validate:"omitempty,notblank,max=32"`
	RuntimePort          *int                          `json:"runtimePort" validate:"omitempty,min=1,max=65535"`
	IsPublic             *bool                         `json:"isPublic"`
	RegistryCredentialID string                        `json:"registryCredentialId" validate:"omitempty,uuid"`
	EnvVars              []createWorkloadEnvVarRequest `json:"envVars" validate:"max=50,dive"`
}

type workloadResponse struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Subdomain      string `json:"subdomain"`
	ImageRef       string `json:"imageRef"`
	TargetReplicas int32  `json:"targetReplicas"`
	PrimaryRegion  string `json:"primaryRegion"`
	RuntimePort    int32  `json:"runtimePort"`
	Status         string `json:"status"`
	IsPublic       bool   `json:"isPublic"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type listWorkloadsResponse struct {
	Workloads []workloadResponse `json:"workloads"`
}

func (s *Server) handleListWorkloads(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	workloads, err := s.workloads.ListWorkloads(request.Context(), user.ID, workspaceID, projectID)
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	items := make([]workloadResponse, 0, len(workloads))
	for _, workload := range workloads {
		items = append(items, workloadResponseFromModel(workload))
	}
	JSON(responseWriter, http.StatusOK, listWorkloadsResponse{Workloads: items})
}

func (s *Server) handleCreateWorkload(responseWriter http.ResponseWriter, request *http.Request) {
	user, ok := RequireCallerUser(responseWriter, request, s.users)
	if !ok {
		return
	}

	workspaceID, projectID, ok := workspaceAndProjectIDFromRequest(request)
	if !ok {
		Error(responseWriter, http.StatusBadRequest, "invalid input")

		return
	}

	var req createWorkloadRequest
	if err := ReadAndValidateJSON(request, &req, false); err != nil {
		WriteJSONError(responseWriter, err)

		return
	}

	envVars := make([]tenant.WorkloadEnvVarInput, 0, len(req.EnvVars))
	for _, item := range req.EnvVars {
		envVars = append(envVars, tenant.WorkloadEnvVarInput{
			Key:      strings.ToUpper(strings.TrimSpace(item.Key)),
			Value:    item.Value,
			IsSecret: item.IsSecret,
		})
	}

	_, country := requestOrigin(request)
	plan, err := s.placement.Resolve(req.PrimaryRegion, country)
	if err != nil {
		if errors.Is(err, placement.ErrUnknownRegion) {
			Error(responseWriter, http.StatusBadRequest, err.Error())
			return
		}
		Error(responseWriter, http.StatusInternalServerError, "internal error")
		return
	}
	selectedRegion := plan.Candidates[0]

	result, err := s.workloads.CreateWorkload(request.Context(), user.ID, workspaceID, projectID, tenant.CreateWorkloadParams{
		Name:     req.Name,
		ImageRef: req.ImageRef,
		Compute: tenant.WorkloadComputeInput{
			VCPU:      req.Compute.VCPU,
			MemoryMB:  req.Compute.MemoryMB,
			Dedicated: req.Compute.Dedicated,
		},
		PrimaryRegion:        selectedRegion,
		RuntimePort:          req.RuntimePort,
		IsPublic:             req.IsPublic,
		RegistryCredentialID: req.RegistryCredentialID,
		EnvVars:              envVars,
	})
	if err != nil {
		WriteTenantError(responseWriter, err)

		return
	}

	workload := result.Workload
	if s.dispatcher != nil {
		dispatchCtx, cancel := newCreateWorkloadDispatchContext(request.Context())
		defer cancel()

		dispatchErr := s.dispatcher.Launch(dispatchCtx, fabric.Request{
			WorkloadID:   result.WorkloadID,
			DeploymentID: result.DeploymentID,
			MicroVMID:    result.MicroVMID,
			WorkspaceID:  workspaceID,
			Region:       selectedRegion,
			Regions:      plan.Candidates,
			Shape:        result.Shape,
			ImageRef:     result.Workload.ImageRef,
			Env:          result.Env,
			RuntimePort:  uint32(result.Workload.RuntimePort),
		})

		if dispatchErr != nil {
			switch {
			case errors.Is(dispatchErr, fabric.ErrNoAuctionBids):
				Error(responseWriter, http.StatusServiceUnavailable, "no capacity available")

				return
			case errors.Is(dispatchErr, fabric.ErrLaunchRejected):
				Error(responseWriter, http.StatusBadGateway, "launch rejected")

				return
			}
		}

		refreshed, refreshErr := s.workloads.GetWorkload(dispatchCtx, user.ID, workspaceID, projectID, workload.ID)
		workload = resolveCreateWorkloadAfterDispatch(workload, dispatchErr, refreshed, refreshErr)
	}

	responseWriter.Header().Set(
		"Location",
		"/v1/workspaces/"+url.PathEscape(workspaceID)+"/projects/"+url.PathEscape(projectID)+"/workloads/"+url.PathEscape(workload.ID),
	)
	JSON(responseWriter, http.StatusCreated, workloadResponseFromModel(workload))
}

func workloadResponseFromModel(workload tenant.Workload) workloadResponse {
	return workloadResponse{
		ID:             workload.ID,
		ProjectID:      workload.ProjectID,
		Name:           workload.Name,
		Slug:           workload.Slug,
		Subdomain:      workload.Subdomain,
		ImageRef:       workload.ImageRef,
		TargetReplicas: workload.TargetReplicas,
		PrimaryRegion:  workload.PrimaryRegion,
		RuntimePort:    workload.RuntimePort,
		Status:         workload.Status,
		IsPublic:       workload.IsPublic,
		CreatedAt:      workload.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      workload.UpdatedAt.Format(time.RFC3339),
	}
}

func newCreateWorkloadDispatchContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), createWorkloadDispatchTimeout)
}

func resolveCreateWorkloadAfterDispatch(current tenant.Workload, dispatchErr error, refreshed tenant.Workload, refreshErr error) tenant.Workload {
	if refreshErr == nil {
		return refreshed
	}
	if dispatchErr == nil {
		current.Status = "deploying"
	}

	return current
}
