/**
 * API Endpoints Configuration
 * ============================
 * All API endpoint paths are defined here.
 * Update these paths to match your backend API structure.
 */

export const API_ENDPOINTS = {
  // Authentication
  auth: {
    exchange: "/v1/auth/exchange",
    sessions: "/v1/sessions",
  },

  // Apps
  apps: {
    list: "/v1/apps",
    get: (appId: string) => `/v1/apps/${appId}`,
    update: (appId: string) => `/v1/apps/${appId}`,
    delete: (appId: string) => `/v1/apps/${appId}`,
    redeploy: (appId: string) => `/v1/apps/${appId}/redeploy`,
    deployments: (appId: string) => `/v1/apps/${appId}/deployments`,
    logs: (appId: string) => `/v1/apps/${appId}/logs`,
  },

  // Deployments
  deployments: {
    create: "/v1/deployments",
    get: (deploymentId: string) => `/v1/deployments/${deploymentId}`,
    cancel: (deploymentId: string) => `/v1/deployments/${deploymentId}/cancel`,
    retry: (deploymentId: string) => `/v1/deployments/${deploymentId}/retry`,
    logs: (deploymentId: string) => `/v1/deployments/${deploymentId}/logs`,
  },

  // Subdomains
  subdomains: {
    check: "/v1/subdomains/check",
  },

  // GitHub (optional, for repo listing)
  github: {
    repos: "/v1/github/repos",
    branches: (owner: string, repo: string) => `/v1/github/repos/${owner}/${repo}/branches`,
  },
} as const;
