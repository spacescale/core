"use client";

import { Github, FileCode, Container } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, Badge } from "@/components/ui";
import { useDeployWizardStore } from "@/stores/deploy-wizard";

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

const sourceIcons = {
  github: Github,
  dockerfile: FileCode,
  docker_image: Container,
};

const planLabels = {
  starter: "Starter",
  standard: "Standard",
  pro: "Pro",
};

export function StepReview() {
  const {
    sourceType,
    repository,
    branch,
    dockerfilePath,
    dockerImage,
    dockerTag,
    buildCommand,
    startCommand,
    port,
    appName,
    subdomain,
    envVars,
    selectedPlan,
  } = useDeployWizardStore();

  const SourceIcon = sourceType ? sourceIcons[sourceType] : Github;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Review & Deploy</h2>
        <p className="text-sm text-muted-foreground">
          Review your configuration before deploying
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {/* App Info */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Application
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div>
              <span className="text-sm text-muted-foreground">Name: </span>
              <span className="font-medium">{appName || "—"}</span>
            </div>
            <div>
              <span className="text-sm text-muted-foreground">URL: </span>
              <span className="font-medium">
                https://{subdomain || "—"}.{PLATFORM_DOMAIN}
              </span>
            </div>
            <div>
              <span className="text-sm text-muted-foreground">Plan: </span>
              <Badge variant="secondary">{planLabels[selectedPlan]}</Badge>
            </div>
          </CardContent>
        </Card>

        {/* Source */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <SourceIcon className="h-4 w-4" />
              Source
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {(sourceType === "github" || sourceType === "dockerfile") && (
              <>
                <div>
                  <span className="text-sm text-muted-foreground">Repository: </span>
                  <span className="font-medium">{repository || "—"}</span>
                </div>
                <div>
                  <span className="text-sm text-muted-foreground">Branch: </span>
                  <span className="font-medium">{branch || "—"}</span>
                </div>
                {sourceType === "dockerfile" && (
                  <div>
                    <span className="text-sm text-muted-foreground">Dockerfile: </span>
                    <span className="font-medium">{dockerfilePath || "Dockerfile"}</span>
                  </div>
                )}
              </>
            )}
            {sourceType === "docker_image" && (
              <div>
                <span className="text-sm text-muted-foreground">Image: </span>
                <span className="font-medium">
                  {dockerImage || "—"}:{dockerTag || "latest"}
                </span>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Build & Runtime */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Build & Runtime
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {buildCommand && (
              <div>
                <span className="text-sm text-muted-foreground">Build: </span>
                <code className="rounded bg-muted px-1 text-sm">{buildCommand}</code>
              </div>
            )}
            {startCommand && (
              <div>
                <span className="text-sm text-muted-foreground">Start: </span>
                <code className="rounded bg-muted px-1 text-sm">{startCommand}</code>
              </div>
            )}
            <div>
              <span className="text-sm text-muted-foreground">Port: </span>
              <span className="font-medium">{port}</span>
            </div>
          </CardContent>
        </Card>

        {/* Environment Variables */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Environment Variables
            </CardTitle>
          </CardHeader>
          <CardContent>
            {envVars.length === 0 ? (
              <p className="text-sm text-muted-foreground">No environment variables</p>
            ) : (
              <div className="space-y-1">
                {envVars
                  .filter((env) => env.key)
                  .map((env, index) => (
                    <div key={index} className="flex items-center gap-2">
                      <code className="rounded bg-muted px-1 text-sm">{env.key}</code>
                      <span className="text-muted-foreground">=</span>
                      <span className="text-sm">••••••••</span>
                    </div>
                  ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
