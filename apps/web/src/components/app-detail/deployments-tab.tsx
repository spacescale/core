"use client";

import Link from "next/link";
import {
  Card,
  CardContent,
  Badge,
  Skeleton,
} from "@/components/ui";
import { useAppDeployments } from "@/lib/hooks";
import { formatRelativeTime, cn } from "@/lib/utils";
import type { Deployment } from "@/lib/schemas";

interface DeploymentsTabProps {
  appId: string;
}

function DeploymentRow({ deployment }: { deployment: Deployment }) {
  return (
    <Link
      href={`/app/deployments/${deployment.id}`}
      className="block rounded-lg border p-4 transition-colors hover:bg-muted/50"
    >
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-medium">v{deployment.version || "—"}</span>
            <Badge variant={deployment.status}>{deployment.status}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {deployment.source.type === "github" &&
              `${deployment.source.repository} (${deployment.source.branch})`}
            {deployment.source.type === "dockerfile" &&
              `${deployment.source.repository}`}
            {deployment.source.type === "docker_image" &&
              `${deployment.source.image}:${deployment.source.tag || "latest"}`}
          </p>
        </div>
        <span className="text-sm text-muted-foreground">
          {formatRelativeTime(deployment.createdAt)}
        </span>
      </div>
      {deployment.error && (
        <p className="mt-2 text-sm text-destructive">{deployment.error}</p>
      )}
    </Link>
  );
}

export function DeploymentsTab({ appId }: DeploymentsTabProps) {
  const { data: deployments, isLoading, error } = useAppDeployments(appId);

  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="rounded-lg border p-4">
            <div className="flex items-center justify-between">
              <div className="space-y-2">
                <Skeleton className="h-5 w-24" />
                <Skeleton className="h-4 w-48" />
              </div>
              <Skeleton className="h-4 w-16" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <Card className="py-8">
        <CardContent className="text-center">
          <p className="text-destructive">Failed to load deployments</p>
        </CardContent>
      </Card>
    );
  }

  if (!deployments || deployments.length === 0) {
    return (
      <Card className="py-8">
        <CardContent className="text-center">
          <p className="text-muted-foreground">No deployments yet</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {deployments.map((deployment) => (
        <DeploymentRow key={deployment.id} deployment={deployment} />
      ))}
    </div>
  );
}
