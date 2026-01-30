import { describe, it, expect } from "vitest";
import {
  AppSchema,
  DeploymentSchema,
  EnvVarSchema,
  safeParse,
} from "@/lib/schemas";

describe("EnvVarSchema", () => {
  it("validates valid env vars", () => {
    const result = safeParse(EnvVarSchema, { key: "API_KEY", value: "secret" });
    expect(result.success).toBe(true);
  });

  it("rejects empty keys", () => {
    const result = safeParse(EnvVarSchema, { key: "", value: "secret" });
    expect(result.success).toBe(false);
  });
});

describe("AppSchema", () => {
  const validApp = {
    id: "app-1",
    name: "My App",
    subdomain: "my-app",
    status: "live",
    url: "https://my-app.example.com",
    source: {
      type: "github",
      repository: "user/repo",
      branch: "main",
    },
    plan: "starter",
    createdAt: "2024-01-01T00:00:00Z",
    updatedAt: "2024-01-02T00:00:00Z",
  };

  it("validates a valid app", () => {
    const result = safeParse(AppSchema, validApp);
    expect(result.success).toBe(true);
  });

  it("rejects invalid status", () => {
    const result = safeParse(AppSchema, { ...validApp, status: "invalid" });
    expect(result.success).toBe(false);
  });

  it("rejects invalid source type", () => {
    const result = safeParse(AppSchema, {
      ...validApp,
      source: { type: "invalid", repository: "x" },
    });
    expect(result.success).toBe(false);
  });

  it("validates docker_image source", () => {
    const result = safeParse(AppSchema, {
      ...validApp,
      source: { type: "docker_image", image: "nginx", tag: "latest" },
    });
    expect(result.success).toBe(true);
  });
});

describe("DeploymentSchema", () => {
  const validDeployment = {
    id: "deploy-1",
    appId: "app-1",
    status: "succeeded",
    source: {
      type: "github",
      repository: "user/repo",
      branch: "main",
    },
    steps: [
      {
        id: "step-1",
        name: "Build",
        status: "completed",
        startedAt: "2024-01-01T00:00:00Z",
        completedAt: "2024-01-01T00:01:00Z",
        error: null,
      },
    ],
    createdAt: "2024-01-01T00:00:00Z",
    startedAt: "2024-01-01T00:00:00Z",
    completedAt: "2024-01-01T00:05:00Z",
    error: null,
  };

  it("validates a valid deployment", () => {
    const result = safeParse(DeploymentSchema, validDeployment);
    expect(result.success).toBe(true);
  });

  it("rejects invalid status", () => {
    const result = safeParse(DeploymentSchema, { ...validDeployment, status: "invalid" });
    expect(result.success).toBe(false);
  });

  it("allows null completedAt for running deployments", () => {
    const result = safeParse(DeploymentSchema, {
      ...validDeployment,
      status: "running",
      completedAt: null,
    });
    expect(result.success).toBe(true);
  });
});

describe("safeParse", () => {
  it("returns success true for valid data", () => {
    const result = safeParse(EnvVarSchema, { key: "KEY", value: "value" });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.key).toBe("KEY");
    }
  });

  it("returns success false for invalid data", () => {
    const result = safeParse(EnvVarSchema, { key: 123, value: "value" });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error).toBeDefined();
    }
  });
});
