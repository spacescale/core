"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { API_ENDPOINTS } from "@/lib/api/endpoints";
import {
  GitHubReposResponseSchema,
  GitHubBranchesResponseSchema,
  safeParse,
  type GitHubReposResponse,
  type GitHubBranchesResponse,
} from "@/lib/schemas";

export function useGitHubRepos() {
  return useQuery({
    queryKey: ["github-repos"],
    queryFn: async () => {
      const data = await api.get<GitHubReposResponse>(API_ENDPOINTS.github.repos);
      const result = safeParse(GitHubReposResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse GitHub repos response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data.repos;
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

export function useGitHubBranches(owner: string, repo: string) {
  return useQuery({
    queryKey: ["github-branches", owner, repo],
    queryFn: async () => {
      const data = await api.get<GitHubBranchesResponse>(
        API_ENDPOINTS.github.branches(owner, repo)
      );
      const result = safeParse(GitHubBranchesResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse GitHub branches response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data.branches;
    },
    enabled: !!owner && !!repo,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
