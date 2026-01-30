"use client";

import { useState } from "react";
import Link from "next/link";
import { ExternalLink, Copy, Check, RefreshCw } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Badge,
} from "@/components/ui";
import { useRedeployApp, toast } from "@/lib/hooks";
import { formatDate, formatRelativeTime, copyToClipboard } from "@/lib/utils";
import type { App } from "@/lib/schemas";

const statusLabels = {
  live: "Live",
  deploying: "Deploying",
  failed: "Failed",
  stopped: "Stopped",
};

const planLabels = {
  starter: "Starter",
  standard: "Standard",
  pro: "Pro",
};

interface OverviewTabProps {
  app: App;
}

export function OverviewTab({ app }: OverviewTabProps) {
  const [copied, setCopied] = useState(false);
  const { mutate: redeploy, isPending } = useRedeployApp(app.id);

  const handleCopyUrl = async () => {
    if (app.url) {
      await copyToClipboard(app.url);
      setCopied(true);
      toast({ title: "URL copied to clipboard" });
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleRedeploy = () => {
    redeploy();
  };

  return (
    <div className="space-y-6">
      {/* Status & URL */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">Status</CardTitle>
            <Badge variant={app.status}>{statusLabels[app.status]}</Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {app.url && (
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">URL:</span>
              <a
                href={app.url}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-primary hover:underline"
              >
                {app.url}
                <ExternalLink className="h-3 w-3" />
              </a>
              <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleCopyUrl}>
                {copied ? (
                  <Check className="h-3 w-3 text-success" />
                ) : (
                  <Copy className="h-3 w-3" />
                )}
              </Button>
            </div>
          )}
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Plan:</span>
            <Badge variant="secondary">{planLabels[app.plan]}</Badge>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleRedeploy}
              disabled={isPending}
              loading={isPending}
            >
              <RefreshCw className="mr-2 h-4 w-4" />
              Redeploy
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Source */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Source</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {app.source.type === "github" && (
            <>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Repository:</span>
                <span>{app.source.repository}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Branch:</span>
                <span>{app.source.branch}</span>
              </div>
            </>
          )}
          {app.source.type === "dockerfile" && (
            <>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Repository:</span>
                <span>{app.source.repository}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Dockerfile:</span>
                <span>{app.source.dockerfilePath || "Dockerfile"}</span>
              </div>
            </>
          )}
          {app.source.type === "docker_image" && (
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Image:</span>
              <span>
                {app.source.image}:{app.source.tag || "latest"}
              </span>
            </div>
          )}
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Port:</span>
            <span>{app.port || 3000}</span>
          </div>
        </CardContent>
      </Card>

      {/* Latest Deployment */}
      {app.latestDeployment && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="text-lg">Latest Deployment</CardTitle>
              <Badge variant={app.latestDeployment.status}>
                {app.latestDeployment.status}
              </Badge>
            </div>
            <CardDescription>
              v{app.latestDeployment.version} - {formatRelativeTime(app.latestDeployment.createdAt)}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button variant="outline" size="sm" asChild>
              <Link href={`/app/deployments/${app.latestDeployment.id}`}>
                View Details
              </Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Timestamps */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Timeline</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Created:</span>
            <span>{formatDate(app.createdAt)}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Last Updated:</span>
            <span>{formatDate(app.updatedAt)}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
