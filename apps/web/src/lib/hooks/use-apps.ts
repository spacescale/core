"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { API_ENDPOINTS } from "@/lib/api/endpoints";
import {
  AppSchema,
  AppsListResponseSchema,
  safeParse,
  type App,
  type AppsListResponse,
  type UpdateAppRequest,
} from "@/lib/schemas";
import { toast } from "@/lib/hooks/use-toast";

export function useApps() {
  return useQuery({
    queryKey: ["apps"],
    queryFn: async () => {
      const data = await api.get<AppsListResponse>(API_ENDPOINTS.apps.list);
      const result = safeParse(AppsListResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse apps response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
  });
}

export function useApp(appId: string) {
  return useQuery({
    queryKey: ["app", appId],
    queryFn: async () => {
      const data = await api.get<App>(API_ENDPOINTS.apps.get(appId));
      const result = safeParse(AppSchema, data);
      if (!result.success) {
        console.error("Failed to parse app response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    enabled: !!appId,
  });
}

export function useUpdateApp(appId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (updates: UpdateAppRequest) => {
      const data = await api.patch<App>(API_ENDPOINTS.apps.update(appId), updates);
      const result = safeParse(AppSchema, data);
      if (!result.success) {
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["app", appId], data);
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast({
        title: "App updated",
        description: "Your changes have been saved.",
      });
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to update app",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}

export function useDeleteApp(appId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      await api.delete(API_ENDPOINTS.apps.delete(appId));
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      queryClient.removeQueries({ queryKey: ["app", appId] });
      toast({
        title: "App deleted",
        description: "The app has been permanently deleted.",
      });
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to delete app",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}

export function useRedeployApp(appId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const data = await api.post(API_ENDPOINTS.apps.redeploy(appId));
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app", appId] });
      queryClient.invalidateQueries({ queryKey: ["apps"] });
      toast({
        title: "Deployment started",
        description: "A new deployment has been queued.",
      });
    },
    onError: (error) => {
      toast({
        variant: "destructive",
        title: "Failed to start deployment",
        description: error instanceof Error ? error.message : "An error occurred",
      });
    },
  });
}
