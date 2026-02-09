import { z } from "zod";

// ===========================================
// Enums
// ===========================================

export const DeploymentStatusSchema = z.enum([
  "queued",
  "running",
  "succeeded",
  "failed",
  "canceled",
]);

export const AppStatusSchema = z.enum([
  "live",
  "deploying",
  "failed",
  "stopped",
]);

export const SourceTypeSchema = z.enum([
  "github",
  "dockerfile",
  "docker_image",
]);

export const PlanSchema = z.enum([
  "starter",
  "standard",
  "pro",
]);

export const DeploymentStepStatusSchema = z.enum([
  "pending",
  "running",
  "completed",
  "failed",
  "skipped",
]);

// ===========================================
// Core Models
// ===========================================

export const UserSessionSchema = z.object({
  id: z.string(),
  email: z.string().email(),
  name: z.string().nullable(),
  image: z.string().url().nullable(),
  accessToken: z.string().optional(),
});

export const EnvVarSchema = z.object({
  key: z.string().min(1),
  value: z.string(),
});

export const AppSourceSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("github"),
    repository: z.string(),
    branch: z.string(),
  }),
  z.object({
    type: z.literal("dockerfile"),
    repository: z.string(),
    branch: z.string(),
    dockerfilePath: z.string().optional(),
  }),
  z.object({
    type: z.literal("docker_image"),
    image: z.string(),
    tag: z.string().optional(),
  }),
]);

export const DeploymentStepSchema = z.object({
  id: z.string(),
  name: z.string(),
  status: DeploymentStepStatusSchema,
  startedAt: z.string().datetime().nullable(),
  completedAt: z.string().datetime().nullable(),
  error: z.string().nullable().optional(),
});

export const DeploymentSchema = z.object({
  id: z.string(),
  appId: z.string(),
  status: DeploymentStatusSchema,
  version: z.number().optional(),
  source: AppSourceSchema,
  steps: z.array(DeploymentStepSchema),
  createdAt: z.string().datetime(),
  startedAt: z.string().datetime().nullable(),
  completedAt: z.string().datetime().nullable(),
  error: z.string().nullable().optional(),
});

export const AppSchema = z.object({
  id: z.string(),
  name: z.string(),
  subdomain: z.string(),
  status: AppStatusSchema,
  url: z.string().url().nullable(),
  source: AppSourceSchema,
  plan: PlanSchema,
  envVars: z.array(EnvVarSchema).optional(),
  buildCommand: z.string().nullable().optional(),
  startCommand: z.string().nullable().optional(),
  port: z.number().optional(),
  latestDeployment: DeploymentSchema.nullable().optional(),
  createdAt: z.string().datetime(),
  updatedAt: z.string().datetime(),
});

// ===========================================
// API Response Schemas
// ===========================================

export const AppsListResponseSchema = z.object({
  apps: z.array(AppSchema),
  total: z.number(),
  page: z.number().optional(),
  pageSize: z.number().optional(),
});

export const LogEntrySchema = z.object({
  timestamp: z.string().datetime(),
  level: z.enum(["info", "warn", "error", "debug"]),
  message: z.string(),
  source: z.string().optional(),
});

export const LogsResponseSchema = z.object({
  logs: z.array(LogEntrySchema),
  cursor: z.string().nullable(),
  hasMore: z.boolean(),
});

export const SubdomainCheckResponseSchema = z.object({
  available: z.boolean(),
  subdomain: z.string(),
  suggestion: z.string().optional(),
});

export const GitHubRepoSchema = z.object({
  id: z.number(),
  name: z.string(),
  fullName: z.string(),
  private: z.boolean(),
  defaultBranch: z.string(),
  url: z.string().url(),
});

export const GitHubBranchSchema = z.object({
  name: z.string(),
  protected: z.boolean(),
});

export const GitHubReposResponseSchema = z.object({
  repos: z.array(GitHubRepoSchema),
});

export const GitHubBranchesResponseSchema = z.object({
  branches: z.array(GitHubBranchSchema),
});

// ===========================================
// Request Schemas
// ===========================================

export const CreateDeploymentRequestSchema = z.object({
  name: z.string().min(1).max(63),
  subdomain: z.string().min(1).max(63),
  source: AppSourceSchema,
  plan: PlanSchema,
  envVars: z.array(EnvVarSchema).optional(),
  buildCommand: z.string().optional(),
  startCommand: z.string().optional(),
  port: z.number().min(1).max(65535).optional(),
});

export const UpdateAppRequestSchema = z.object({
  name: z.string().min(1).max(63).optional(),
  subdomain: z.string().min(1).max(63).optional(),
  envVars: z.array(EnvVarSchema).optional(),
  buildCommand: z.string().optional(),
  startCommand: z.string().optional(),
  port: z.number().min(1).max(65535).optional(),
});

// ===========================================
// Type Exports
// ===========================================

export type DeploymentStatus = z.infer<typeof DeploymentStatusSchema>;
export type AppStatus = z.infer<typeof AppStatusSchema>;
export type SourceType = z.infer<typeof SourceTypeSchema>;
export type Plan = z.infer<typeof PlanSchema>;
export type DeploymentStepStatus = z.infer<typeof DeploymentStepStatusSchema>;

export type UserSession = z.infer<typeof UserSessionSchema>;
export type EnvVar = z.infer<typeof EnvVarSchema>;
export type AppSource = z.infer<typeof AppSourceSchema>;
export type DeploymentStep = z.infer<typeof DeploymentStepSchema>;
export type Deployment = z.infer<typeof DeploymentSchema>;
export type App = z.infer<typeof AppSchema>;

export type AppsListResponse = z.infer<typeof AppsListResponseSchema>;
export type LogEntry = z.infer<typeof LogEntrySchema>;
export type LogsResponse = z.infer<typeof LogsResponseSchema>;
export type SubdomainCheckResponse = z.infer<typeof SubdomainCheckResponseSchema>;
export type GitHubRepo = z.infer<typeof GitHubRepoSchema>;
export type GitHubBranch = z.infer<typeof GitHubBranchSchema>;
export type GitHubReposResponse = z.infer<typeof GitHubReposResponseSchema>;
export type GitHubBranchesResponse = z.infer<typeof GitHubBranchesResponseSchema>;

export type CreateDeploymentRequest = z.infer<typeof CreateDeploymentRequestSchema>;
export type UpdateAppRequest = z.infer<typeof UpdateAppRequestSchema>;

// ===========================================
// Safe Parse Utility
// ===========================================

export function safeParse<T>(
  schema: z.ZodSchema<T>,
  data: unknown
): { success: true; data: T } | { success: false; error: z.ZodError } {
  const result = schema.safeParse(data);
  return result;
}

export function parseOrThrow<T>(schema: z.ZodSchema<T>, data: unknown): T {
  return schema.parse(data);
}
