"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { API_ENDPOINTS } from "@/lib/api/endpoints";
import {
  SubdomainCheckResponseSchema,
  safeParse,
  type SubdomainCheckResponse,
} from "@/lib/schemas";
import { isValidSubdomain } from "@/lib/utils";

export function useSubdomainCheck(subdomain: string) {
  return useQuery({
    queryKey: ["subdomain-check", subdomain],
    queryFn: async () => {
      const data = await api.get<SubdomainCheckResponse>(API_ENDPOINTS.subdomains.check, {
        name: subdomain,
      });
      const result = safeParse(SubdomainCheckResponseSchema, data);
      if (!result.success) {
        console.error("Failed to parse subdomain check response:", result.error);
        throw new Error("Unexpected server response");
      }
      return result.data;
    },
    enabled: subdomain.length > 0 && isValidSubdomain(subdomain),
    staleTime: 10000, // 10 seconds
  });
}
