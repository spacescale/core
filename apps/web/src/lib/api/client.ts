import { getSession } from "next-auth/react";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8080";
const USE_MOCK_API = process.env.NEXT_PUBLIC_USE_MOCK_API === "true";

export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public data?: unknown
  ) {
    super(`API Error: ${status} ${statusText}`);
    this.name = "ApiError";
  }
}

export interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  params?: Record<string, string | number | boolean | undefined>;
}

async function getAuthHeaders(): Promise<HeadersInit> {
  const session = await getSession();
  const headers: HeadersInit = {
    "Content-Type": "application/json",
  };

  if (session?.accessToken) {
    headers["Authorization"] = `Bearer ${session.accessToken}`;
  }

  return headers;
}

function buildUrl(path: string, params?: Record<string, string | number | boolean | undefined>): string {
  const url = new URL(path, API_BASE_URL);

  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        url.searchParams.append(key, String(value));
      }
    });
  }

  return url.toString();
}

export async function apiClient<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { body, params, ...init } = options;

  // In mock mode, import and use mock handlers
  if (USE_MOCK_API) {
    const { mockApiClient } = await import("@/mocks/client");
    return mockApiClient<T>(path, { ...init, body, params });
  }

  const url = buildUrl(path, params);
  const headers = await getAuthHeaders();

  const response = await fetch(url, {
    ...init,
    headers: {
      ...headers,
      ...init.headers,
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    let errorData: unknown;
    try {
      errorData = await response.json();
    } catch {
      // Response is not JSON
    }
    throw new ApiError(response.status, response.statusText, errorData);
  }

  // Handle empty responses
  const text = await response.text();
  if (!text) {
    return {} as T;
  }

  return JSON.parse(text) as T;
}

// Convenience methods
export const api = {
  get: <T>(path: string, params?: Record<string, string | number | boolean | undefined>) =>
    apiClient<T>(path, { method: "GET", params }),

  post: <T>(path: string, body?: unknown) =>
    apiClient<T>(path, { method: "POST", body }),

  patch: <T>(path: string, body?: unknown) =>
    apiClient<T>(path, { method: "PATCH", body }),

  put: <T>(path: string, body?: unknown) =>
    apiClient<T>(path, { method: "PUT", body }),

  delete: <T>(path: string) =>
    apiClient<T>(path, { method: "DELETE" }),
};
