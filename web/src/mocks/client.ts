import type { RequestOptions } from "@/lib/api/client";
import { API_ENDPOINTS } from "@/lib/api/endpoints";
import {
  mockApps,
  mockDeployments,
  mockGitHubRepos,
  mockBranches,
  usedSubdomains,
  generateMockLogs,
  createQueuedSteps,
} from "./data";
import type { App, Deployment, CreateDeploymentRequest } from "@/lib/schemas";

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

// In-memory state for mock mode (simulates DB)
const apps = [...mockApps];
const deployments = [...mockDeployments];
let deploymentIdCounter = 100;
let appIdCounter = 100;

// Deployment simulation state
const simulatingDeployments = new Map<string, NodeJS.Timeout>();

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function matchPath(
  path: string,
  pattern: string
): { match: boolean; params: Record<string, string> } {
  const pathParts = path.split("/").filter(Boolean);
  const patternParts = pattern.split("/").filter(Boolean);

  if (pathParts.length !== patternParts.length) {
    return { match: false, params: {} };
  }

  const params: Record<string, string> = {};

  for (let i = 0; i < patternParts.length; i++) {
    if (patternParts[i].startsWith(":")) {
      params[patternParts[i].slice(1)] = pathParts[i];
    } else if (patternParts[i] !== pathParts[i]) {
      return { match: false, params: {} };
    }
  }

  return { match: true, params };
}

function simulateDeploymentProgress(deploymentId: string) {
  let stepIndex = 0;
  const stepDurations = [2000, 3000, 2000, 2000, 4000, 2000, 3000, 1000]; // ms per step

  const interval = setInterval(() => {
    const deployment = deployments.find((d) => d.id === deploymentId);
    if (!deployment) {
      clearInterval(interval);
      simulatingDeployments.delete(deploymentId);
      return;
    }

    if (deployment.status === "canceled") {
      clearInterval(interval);
      simulatingDeployments.delete(deploymentId);
      return;
    }

    // Complete current step
    if (stepIndex < deployment.steps.length) {
      deployment.steps[stepIndex].status = "completed";
      deployment.steps[stepIndex].completedAt = new Date().toISOString();

      stepIndex++;

      // Start next step
      if (stepIndex < deployment.steps.length) {
        deployment.steps[stepIndex].status = "running";
        deployment.steps[stepIndex].startedAt = new Date().toISOString();
      }
    }

    // Check if deployment is complete
    if (stepIndex >= deployment.steps.length) {
      deployment.status = "succeeded";
      deployment.completedAt = new Date().toISOString();

      // Update app status
      const app = apps.find((a) => a.id === deployment.appId);
      if (app) {
        app.status = "live";
        app.url = `https://${app.subdomain}.${PLATFORM_DOMAIN}`;
        app.latestDeployment = deployment;
      }

      clearInterval(interval);
      simulatingDeployments.delete(deploymentId);
    }
  }, stepDurations[stepIndex] || 2000);

  simulatingDeployments.set(deploymentId, interval);
}

export async function mockApiClient<T>(
  path: string,
  options: RequestOptions & { body?: unknown; params?: Record<string, string | number | boolean | undefined> }
): Promise<T> {
  await delay(300 + Math.random() * 200); // Simulate network latency

  const { method = "GET", body, params } = options;

  // Parse query params from path if present
  const [basePath, queryString] = path.split("?");
  const queryParams = new URLSearchParams(queryString || "");
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) queryParams.set(key, String(value));
    });
  }

  // ===========================================
  // Apps Endpoints
  // ===========================================

  if (basePath === API_ENDPOINTS.apps.list && method === "GET") {
    return { apps, total: apps.length } as T;
  }

  const getAppMatch = matchPath(basePath, "/v1/apps/:appId");
  if (getAppMatch.match && method === "GET") {
    const app = apps.find((a) => a.id === getAppMatch.params.appId);
    if (!app) throw new Error("App not found");
    return app as T;
  }

  if (getAppMatch.match && method === "PATCH") {
    const app = apps.find((a) => a.id === getAppMatch.params.appId);
    if (!app) throw new Error("App not found");
    const updates = body as Partial<App>;
    Object.assign(app, updates, { updatedAt: new Date().toISOString() });
    return app as T;
  }

  if (getAppMatch.match && method === "DELETE") {
    const index = apps.findIndex((a) => a.id === getAppMatch.params.appId);
    if (index === -1) throw new Error("App not found");
    apps.splice(index, 1);
    return {} as T;
  }

  const appDeploymentsMatch = matchPath(basePath, "/v1/apps/:appId/deployments");
  if (appDeploymentsMatch.match && method === "GET") {
    const appDeployments = deployments.filter(
      (d) => d.appId === appDeploymentsMatch.params.appId
    );
    return { deployments: appDeployments } as T;
  }

  const appLogsMatch = matchPath(basePath, "/v1/apps/:appId/logs");
  if (appLogsMatch.match && method === "GET") {
    return {
      logs: generateMockLogs(20),
      cursor: null,
      hasMore: false,
    } as T;
  }

  const redeployMatch = matchPath(basePath, "/v1/apps/:appId/redeploy");
  if (redeployMatch.match && method === "POST") {
    const app = apps.find((a) => a.id === redeployMatch.params.appId);
    if (!app) throw new Error("App not found");

    const newDeployment: Deployment = {
      id: `deploy-${++deploymentIdCounter}`,
      appId: app.id,
      status: "queued",
      version: (app.latestDeployment?.version || 0) + 1,
      source: app.source,
      steps: createQueuedSteps(),
      createdAt: new Date().toISOString(),
      startedAt: null,
      completedAt: null,
      error: null,
    };

    deployments.push(newDeployment);
    app.status = "deploying";
    app.latestDeployment = newDeployment;

    // Start simulation
    simulateDeploymentProgress(newDeployment.id);

    return newDeployment as T;
  }

  // ===========================================
  // Deployments Endpoints
  // ===========================================

  if (basePath === API_ENDPOINTS.deployments.create && method === "POST") {
    const data = body as CreateDeploymentRequest;

    // Create new app
    const newApp: App = {
      id: `app-${++appIdCounter}`,
      name: data.name,
      subdomain: data.subdomain,
      status: "deploying",
      url: null,
      source: data.source,
      plan: data.plan,
      envVars: data.envVars || [],
      buildCommand: data.buildCommand || null,
      startCommand: data.startCommand || null,
      port: data.port || 3000,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      latestDeployment: null,
    };

    // Create deployment
    const newDeployment: Deployment = {
      id: `deploy-${++deploymentIdCounter}`,
      appId: newApp.id,
      status: "queued",
      version: 1,
      source: data.source,
      steps: createQueuedSteps(),
      createdAt: new Date().toISOString(),
      startedAt: null,
      completedAt: null,
      error: null,
    };

    newApp.latestDeployment = newDeployment;
    apps.push(newApp);
    deployments.push(newDeployment);
    usedSubdomains.add(data.subdomain);

    // Start simulation
    simulateDeploymentProgress(newDeployment.id);

    return { app: newApp, deployment: newDeployment } as T;
  }

  const getDeploymentMatch = matchPath(basePath, "/v1/deployments/:deploymentId");
  if (getDeploymentMatch.match && method === "GET") {
    const deployment = deployments.find(
      (d) => d.id === getDeploymentMatch.params.deploymentId
    );
    if (!deployment) throw new Error("Deployment not found");
    return deployment as T;
  }

  const cancelDeploymentMatch = matchPath(basePath, "/v1/deployments/:deploymentId/cancel");
  if (cancelDeploymentMatch.match && method === "POST") {
    const deployment = deployments.find(
      (d) => d.id === cancelDeploymentMatch.params.deploymentId
    );
    if (!deployment) throw new Error("Deployment not found");

    // Stop simulation
    const timer = simulatingDeployments.get(deployment.id);
    if (timer) {
      clearInterval(timer);
      simulatingDeployments.delete(deployment.id);
    }

    deployment.status = "canceled";
    deployment.completedAt = new Date().toISOString();

    // Update app
    const app = apps.find((a) => a.id === deployment.appId);
    if (app) {
      app.status = "stopped";
      app.latestDeployment = deployment;
    }

    return deployment as T;
  }

  const retryDeploymentMatch = matchPath(basePath, "/v1/deployments/:deploymentId/retry");
  if (retryDeploymentMatch.match && method === "POST") {
    const oldDeployment = deployments.find(
      (d) => d.id === retryDeploymentMatch.params.deploymentId
    );
    if (!oldDeployment) throw new Error("Deployment not found");

    const newDeployment: Deployment = {
      id: `deploy-${++deploymentIdCounter}`,
      appId: oldDeployment.appId,
      status: "queued",
      version: (oldDeployment.version || 0) + 1,
      source: oldDeployment.source,
      steps: createQueuedSteps(),
      createdAt: new Date().toISOString(),
      startedAt: null,
      completedAt: null,
      error: null,
    };

    deployments.push(newDeployment);

    const app = apps.find((a) => a.id === oldDeployment.appId);
    if (app) {
      app.status = "deploying";
      app.latestDeployment = newDeployment;
    }

    // Start simulation
    simulateDeploymentProgress(newDeployment.id);

    return newDeployment as T;
  }

  const deploymentLogsMatch = matchPath(basePath, "/v1/deployments/:deploymentId/logs");
  if (deploymentLogsMatch.match && method === "GET") {
    return {
      logs: generateMockLogs(30),
      cursor: null,
      hasMore: false,
    } as T;
  }

  // ===========================================
  // Subdomain Check
  // ===========================================

  if (basePath === API_ENDPOINTS.subdomains.check && method === "GET") {
    const subdomain = queryParams.get("name") || "";
    const available = !usedSubdomains.has(subdomain);
    return {
      available,
      subdomain,
      suggestion: available ? undefined : `${subdomain}-${Math.floor(Math.random() * 100)}`,
    } as T;
  }

  // ===========================================
  // GitHub Endpoints
  // ===========================================

  if (basePath === API_ENDPOINTS.github.repos && method === "GET") {
    return { repos: mockGitHubRepos } as T;
  }

  const branchesMatch = matchPath(basePath, "/v1/github/repos/:owner/:repo/branches");
  if (branchesMatch.match && method === "GET") {
    return { branches: mockBranches } as T;
  }

  // ===========================================
  // Fallback
  // ===========================================

  console.warn(`[Mock API] Unhandled request: ${method} ${path}`);
  throw new Error(`Mock API: Unhandled request ${method} ${path}`);
}
