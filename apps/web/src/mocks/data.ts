import type { App, Deployment, DeploymentStep, LogEntry, GitHubRepo, GitHubBranch } from "@/lib/schemas";

// ===========================================
// Mock Data Store
// ===========================================

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

// Mock GitHub Repos
export const mockGitHubRepos: GitHubRepo[] = [
  {
    id: 1,
    name: "my-nextjs-app",
    fullName: "user/my-nextjs-app",
    private: false,
    defaultBranch: "main",
    url: "https://github.com/user/my-nextjs-app",
  },
  {
    id: 2,
    name: "express-api",
    fullName: "user/express-api",
    private: true,
    defaultBranch: "main",
    url: "https://github.com/user/express-api",
  },
  {
    id: 3,
    name: "python-flask-app",
    fullName: "user/python-flask-app",
    private: false,
    defaultBranch: "develop",
    url: "https://github.com/user/python-flask-app",
  },
];

export const mockBranches: GitHubBranch[] = [
  { name: "main", protected: true },
  { name: "develop", protected: false },
  { name: "feature/new-ui", protected: false },
  { name: "staging", protected: false },
];

// Mock Apps
export const mockApps: App[] = [
  {
    id: "app-1",
    name: "Production API",
    subdomain: "api",
    status: "live",
    url: `https://api.${PLATFORM_DOMAIN}`,
    source: {
      type: "github",
      repository: "user/express-api",
      branch: "main",
    },
    plan: "standard",
    port: 3000,
    envVars: [
      { key: "NODE_ENV", value: "production" },
      { key: "DATABASE_URL", value: "postgres://..." },
    ],
    createdAt: "2024-01-15T10:00:00Z",
    updatedAt: "2024-01-20T14:30:00Z",
    latestDeployment: {
      id: "deploy-1",
      appId: "app-1",
      status: "succeeded",
      version: 5,
      source: {
        type: "github",
        repository: "user/express-api",
        branch: "main",
      },
      steps: createCompletedSteps(),
      createdAt: "2024-01-20T14:00:00Z",
      startedAt: "2024-01-20T14:00:05Z",
      completedAt: "2024-01-20T14:05:00Z",
      error: null,
    },
  },
  {
    id: "app-2",
    name: "Frontend App",
    subdomain: "app",
    status: "deploying",
    url: null,
    source: {
      type: "github",
      repository: "user/my-nextjs-app",
      branch: "main",
    },
    plan: "starter",
    port: 3000,
    createdAt: "2024-01-18T09:00:00Z",
    updatedAt: "2024-01-21T10:00:00Z",
    latestDeployment: {
      id: "deploy-2",
      appId: "app-2",
      status: "running",
      version: 2,
      source: {
        type: "github",
        repository: "user/my-nextjs-app",
        branch: "main",
      },
      steps: createRunningSteps(),
      createdAt: "2024-01-21T10:00:00Z",
      startedAt: "2024-01-21T10:00:05Z",
      completedAt: null,
      error: null,
    },
  },
  {
    id: "app-3",
    name: "Staging Backend",
    subdomain: "staging-api",
    status: "failed",
    url: null,
    source: {
      type: "docker_image",
      image: "myregistry/backend",
      tag: "latest",
    },
    plan: "starter",
    port: 8080,
    createdAt: "2024-01-10T08:00:00Z",
    updatedAt: "2024-01-19T16:00:00Z",
    latestDeployment: {
      id: "deploy-3",
      appId: "app-3",
      status: "failed",
      version: 3,
      source: {
        type: "docker_image",
        image: "myregistry/backend",
        tag: "latest",
      },
      steps: createFailedSteps(),
      createdAt: "2024-01-19T16:00:00Z",
      startedAt: "2024-01-19T16:00:05Z",
      completedAt: "2024-01-19T16:03:00Z",
      error: "Health check failed after 3 retries",
    },
  },
];

// Mock Deployments (historical)
export const mockDeployments: Deployment[] = [
  ...mockApps.map((app) => app.latestDeployment!),
  {
    id: "deploy-1-old",
    appId: "app-1",
    status: "succeeded",
    version: 4,
    source: {
      type: "github",
      repository: "user/express-api",
      branch: "main",
    },
    steps: createCompletedSteps(),
    createdAt: "2024-01-19T12:00:00Z",
    startedAt: "2024-01-19T12:00:05Z",
    completedAt: "2024-01-19T12:04:30Z",
    error: null,
  },
];

// Used subdomains for availability check
export const usedSubdomains = new Set(mockApps.map((app) => app.subdomain));

// ===========================================
// Step Generators
// ===========================================

function createCompletedSteps(): DeploymentStep[] {
  return [
    { id: "step-1", name: "Queued", status: "completed", startedAt: null, completedAt: "2024-01-20T14:00:05Z", error: null },
    { id: "step-2", name: "Provisioning compute", status: "completed", startedAt: "2024-01-20T14:00:05Z", completedAt: "2024-01-20T14:01:00Z", error: null },
    { id: "step-3", name: "Configuring routing", status: "completed", startedAt: "2024-01-20T14:01:00Z", completedAt: "2024-01-20T14:01:30Z", error: null },
    { id: "step-4", name: "Configuring DNS", status: "completed", startedAt: "2024-01-20T14:01:30Z", completedAt: "2024-01-20T14:02:00Z", error: null },
    { id: "step-5", name: "Pulling image", status: "completed", startedAt: "2024-01-20T14:02:00Z", completedAt: "2024-01-20T14:03:30Z", error: null },
    { id: "step-6", name: "Starting container", status: "completed", startedAt: "2024-01-20T14:03:30Z", completedAt: "2024-01-20T14:04:00Z", error: null },
    { id: "step-7", name: "Health check", status: "completed", startedAt: "2024-01-20T14:04:00Z", completedAt: "2024-01-20T14:04:30Z", error: null },
    { id: "step-8", name: "Live", status: "completed", startedAt: "2024-01-20T14:04:30Z", completedAt: "2024-01-20T14:05:00Z", error: null },
  ];
}

function createRunningSteps(): DeploymentStep[] {
  return [
    { id: "step-1", name: "Queued", status: "completed", startedAt: null, completedAt: "2024-01-21T10:00:05Z", error: null },
    { id: "step-2", name: "Provisioning compute", status: "completed", startedAt: "2024-01-21T10:00:05Z", completedAt: "2024-01-21T10:01:00Z", error: null },
    { id: "step-3", name: "Configuring routing", status: "completed", startedAt: "2024-01-21T10:01:00Z", completedAt: "2024-01-21T10:01:30Z", error: null },
    { id: "step-4", name: "Configuring DNS", status: "running", startedAt: "2024-01-21T10:01:30Z", completedAt: null, error: null },
    { id: "step-5", name: "Pulling image", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-6", name: "Starting container", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-7", name: "Health check", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-8", name: "Live", status: "pending", startedAt: null, completedAt: null, error: null },
  ];
}

function createFailedSteps(): DeploymentStep[] {
  return [
    { id: "step-1", name: "Queued", status: "completed", startedAt: null, completedAt: "2024-01-19T16:00:05Z", error: null },
    { id: "step-2", name: "Provisioning compute", status: "completed", startedAt: "2024-01-19T16:00:05Z", completedAt: "2024-01-19T16:01:00Z", error: null },
    { id: "step-3", name: "Configuring routing", status: "completed", startedAt: "2024-01-19T16:01:00Z", completedAt: "2024-01-19T16:01:30Z", error: null },
    { id: "step-4", name: "Configuring DNS", status: "completed", startedAt: "2024-01-19T16:01:30Z", completedAt: "2024-01-19T16:02:00Z", error: null },
    { id: "step-5", name: "Pulling image", status: "completed", startedAt: "2024-01-19T16:02:00Z", completedAt: "2024-01-19T16:02:30Z", error: null },
    { id: "step-6", name: "Starting container", status: "completed", startedAt: "2024-01-19T16:02:30Z", completedAt: "2024-01-19T16:02:45Z", error: null },
    { id: "step-7", name: "Health check", status: "failed", startedAt: "2024-01-19T16:02:45Z", completedAt: "2024-01-19T16:03:00Z", error: "Container not responding on port 8080" },
    { id: "step-8", name: "Live", status: "skipped", startedAt: null, completedAt: null, error: null },
  ];
}

export function createQueuedSteps(): DeploymentStep[] {
  return [
    { id: "step-1", name: "Queued", status: "running", startedAt: new Date().toISOString(), completedAt: null, error: null },
    { id: "step-2", name: "Provisioning compute", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-3", name: "Configuring routing", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-4", name: "Configuring DNS", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-5", name: "Pulling image", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-6", name: "Starting container", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-7", name: "Health check", status: "pending", startedAt: null, completedAt: null, error: null },
    { id: "step-8", name: "Live", status: "pending", startedAt: null, completedAt: null, error: null },
  ];
}

// ===========================================
// Log Generator
// ===========================================

export function generateMockLogs(count: number = 20): LogEntry[] {
  const messages = [
    "Starting deployment process...",
    "Pulling base image from registry",
    "Installing dependencies...",
    "npm install completed successfully",
    "Building application...",
    "Build completed in 45s",
    "Creating container...",
    "Container created successfully",
    "Configuring network settings",
    "Starting application server",
    "Server listening on port 3000",
    "Health check passed",
    "Deployment completed successfully",
    "Application is now live",
  ];

  const levels: Array<"info" | "warn" | "error" | "debug"> = ["info", "info", "info", "debug", "warn"];
  const now = Date.now();

  return Array.from({ length: count }, (_, i) => ({
    timestamp: new Date(now - (count - i) * 1000).toISOString(),
    level: levels[Math.floor(Math.random() * levels.length)],
    message: messages[i % messages.length],
    source: "deployment",
  }));
}
