"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { API_ENDPOINTS } from "@/lib/api/endpoints";
import {
  DeploymentSchema,
  LogsResponseSchema,
  safeParse,
  type Deployment,
  type LogsResponse,
  type CreateDeploymentRequest,
} from "@/lib/schemas";
import { toast } from "@/lib/hooks/use-toast";

export function useDeployment(deploymentId: string) {
  return useQuery({
    queryKey: ["deployment", deploymentId],
    queryFn: async () => {
      const data = await api.get<Deployment>(API_ENDPOINTS.deployments.get(deploymentId));
      const result = safeParse(DeploymentSchema, data);
      if (!result.success) {
        console.error("Failed to parse deployment response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    enabled: !!deploymentId,
    refetchInterval: (query) => {
      const data = query.state.data;
      // Poll every 3 seconds while deployment is in progress
      if (data?.status === "queued" || data?.status === "running") {
        return 3000;
      }
      return false;
    },
  });
}

export function useAppDeployments(appId: string) {
  return useQuery({
    queryKey: ["app-deployments", appId],
    queryFn: async () => {
      const data = await api.get<{ deployments: Deployment[] }>(
        API_ENDPOINTS.apps.deployments(appId)
      );
      return data.deployments;
    },
    enabled: !!appId,
  });
}

export function useCreateDeployment() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: CreateDeploymentRequest) => {
      const data = await api.post<{ app: unknown; deployment: Deployment }>(
        API_ENDPOINTS.deployments.create,
        request
      );
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast({
        title: "Deployment started",
        description: "Your app is being deployed.",
      });
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to create deployment",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}

export function useCancelDeployment(deploymentId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const data = await api.post<Deployment>(API_ENDPOINTS.deployments.cancel(deploymentId));
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployment", deploymentId] });
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast({
        title: "Deployment canceled",
        description: "The deployment has been canceled.",
      });
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to cancel deployment",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}

export function useRetryDeployment(deploymentId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const data = await api.post<Deployment>(API_ENDPOINTS.deployments.retry(deploymentId));
      return data;
    },
    onSuccess: (newDeployment) => {
      queryClient.invalidateQueries({ queryKey: ["deployment", deploymentId] });
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast({
        title: "Deployment retried",
        description: "A new deployment has been started.",
      });
      return newDeployment;
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to retry deployment",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}

export function useDeploymentLogs(deploymentId: string, cursor?: string) {
  return useQuery({
    queryKey: ["deployment-logs", deploymentId, cursor],
    queryFn: async () => {
      const data = await api.get<LogsResponse>(API_ENDPOINTS.deployments.logs(deploymentId), {
        cursor,
      });
      const result = safeParse(LogsResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse logs response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    enabled: !!deploymentId,
    refetchInterval: 5000, // Poll logs every 5 seconds
  });
}

export function useAppLogs(appId: string, cursor?: string) {
  return useQuery({
    queryKey: ["app-logs", appId, cursor],
    queryFn: async () => {
      const data = await api.get<LogsResponse>(API_ENDPOINTS.apps.logs(appId), { cursor });
      const result = safeParse(LogsResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse logs response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    enabled: !!appId,
    refetchInterval: 5000,
  });
}
