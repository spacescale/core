"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ExternalLink,
  Copy,
  Check,
  XCircle,
  RefreshCw,
  Loader2,
  CheckCircle2,
  Clock,
  AlertCircle,
  Ban,
} from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Badge,
  Skeleton,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui";
import { PageHeader } from "@/components/layout";
import {
  useDeployment,
  useDeploymentLogs,
  useCancelDeployment,
  useRetryDeployment,
  toast,
} from "@/lib/hooks";
import { formatDate, formatRelativeTime, copyToClipboard, cn } from "@/lib/utils";
import type { DeploymentStep, DeploymentStepStatus } from "@/lib/schemas";

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

const statusConfig = {
  queued: { label: "Queued", color: "queued" as const },
  running: { label: "Running", color: "running" as const },
  succeeded: { label: "Succeeded", color: "succeeded" as const },
  failed: { label: "Failed", color: "failed" as const },
  canceled: { label: "Canceled", color: "canceled" as const },
};

const stepStatusIcons: Record<DeploymentStepStatus, React.ComponentType<{ className?: string }>> = {
  pending: Clock,
  running: Loader2,
  completed: CheckCircle2,
  failed: XCircle,
  skipped: Ban,
};

function DeploymentTimeline({ steps }: { steps: DeploymentStep[] }) {
  return (
    <div className="space-y-4">
      {steps.map((step, index) => {
        const Icon = stepStatusIcons[step.status];
        const isLast = index === steps.length - 1;

        return (
          <div key={step.id} className="relative flex gap-4">
            {/* Timeline line */}
            {!isLast && (
              <div
                className={cn(
                  "absolute left-[15px] top-8 h-full w-0.5",
                  step.status === "completed" ? "bg-success" : "bg-muted"
                )}
              />
            )}

            {/* Icon */}
            <div
              className={cn(
                "relative z-10 flex h-8 w-8 shrink-0 items-center justify-center rounded-full",
                step.status === "completed" && "bg-success/10 text-success",
                step.status === "running" && "bg-primary/10 text-primary",
                step.status === "pending" && "bg-muted text-muted-foreground",
                step.status === "failed" && "bg-destructive/10 text-destructive",
                step.status === "skipped" && "bg-muted text-muted-foreground"
              )}
            >
              <Icon
                className={cn(
                  "h-4 w-4",
                  step.status === "running" && "animate-spin"
                )}
              />
            </div>

            {/* Content */}
            <div className="flex-1 pb-4">
              <div className="flex items-center justify-between">
                <span
                  className={cn(
                    "font-medium",
                    step.status === "pending" && "text-muted-foreground"
                  )}
                >
                  {step.name}
                </span>
                {step.completedAt && (
                  <span className="text-xs text-muted-foreground">
                    {formatRelativeTime(step.completedAt)}
                  </span>
                )}
              </div>
              {step.error && (
                <p className="mt-1 text-sm text-destructive">{step.error}</p>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function LogViewer({ deploymentId }: { deploymentId: string }) {
  const { data, isLoading } = useDeploymentLogs(deploymentId);

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-4 w-full" />
        ))}
      </div>
    );
  }

  if (!data?.logs || data.logs.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No logs available yet</p>
    );
  }

  return (
    <div className="max-h-64 overflow-auto rounded-lg bg-muted/50 p-4 font-mono text-xs scrollbar-thin">
      {data.logs.map((log, index) => (
        <div
          key={index}
          className={cn(
            "py-0.5",
            log.level === "error" && "text-destructive",
            log.level === "warn" && "text-warning"
          )}
        >
          <span className="text-muted-foreground">
            [{new Date(log.timestamp).toLocaleTimeString()}]
          </span>{" "}
          {log.message}
        </div>
      ))}
    </div>
  );
}

export default function DeploymentProgressPage() {
  const params = useParams();
  const router = useRouter();
  const deploymentId = params.deploymentId as string;

  const { data: deployment, isLoading, error } = useDeployment(deploymentId);
  const { mutate: cancelDeployment, isPending: isCanceling } = useCancelDeployment(deploymentId);
  const { mutate: retryDeployment, isPending: isRetrying } = useRetryDeployment(deploymentId);

  const [showCancelDialog, setShowCancelDialog] = useState(false);
  const [copied, setCopied] = useState(false);

  const isInProgress = deployment?.status === "queued" || deployment?.status === "running";
  const canCancel = isInProgress;
  const canRetry = deployment?.status === "failed";
  const canRedeploy = deployment?.status === "succeeded";

  const appUrl = deployment?.status === "succeeded"
    ? `https://${deployment.source.type === "docker_image" ? "app" : deployment.appId}.${PLATFORM_DOMAIN}`
    : null;

  const handleCopyUrl = async () => {
    if (appUrl) {
      await copyToClipboard(appUrl);
      setCopied(true);
      toast({ title: "URL copied to clipboard" });
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleCancel = () => {
    cancelDeployment(undefined, {
      onSuccess: () => setShowCancelDialog(false),
    });
  };

  const handleRetry = () => {
    retryDeployment(undefined, {
      onSuccess: (newDeployment) => {
        if (newDeployment?.id) {
          router.push(`/app/deployments/${newDeployment.id}`);
        }
      },
    });
  };

  if (isLoading) {
    return (
      <>
        <PageHeader title="Deployment" />
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-6 w-24 rounded-full" />
            </div>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-4">
              {Array.from({ length: 8 }).map((_, i) => (
                <div key={i} className="flex gap-4">
                  <Skeleton className="h-8 w-8 rounded-full" />
                  <div className="flex-1 space-y-2">
                    <Skeleton className="h-4 w-32" />
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </>
    );
  }

  if (error || !deployment) {
    return (
      <>
        <PageHeader title="Deployment" />
        <Card className="py-12">
          <CardContent className="text-center">
            <AlertCircle className="mx-auto h-12 w-12 text-destructive" />
            <h3 className="mt-4 text-lg font-semibold">Deployment not found</h3>
            <p className="mt-2 text-muted-foreground">
              The deployment you are looking for does not exist or has been deleted.
            </p>
            <Button asChild className="mt-6">
              <Link href="/app">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Dashboard
              </Link>
            </Button>
          </CardContent>
        </Card>
      </>
    );
  }

  const statusInfo = statusConfig[deployment.status];

  return (
    <>
      <PageHeader title="Deployment Progress">
        <Button variant="outline" asChild>
          <Link href={`/app/apps/${deployment.appId}`}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to App
          </Link>
        </Button>
      </PageHeader>

      <div className="space-y-6">
        {/* Status Header */}
        <Card>
          <CardHeader>
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-3">
                <Badge variant={statusInfo.color} className="text-sm">
                  {statusInfo.label}
                </Badge>
                {deployment.version && (
                  <span className="text-sm text-muted-foreground">
                    v{deployment.version}
                  </span>
                )}
                <span className="text-sm text-muted-foreground">
                  Started {formatRelativeTime(deployment.createdAt)}
                </span>
              </div>
              <div className="flex gap-2">
                {canCancel && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setShowCancelDialog(true)}
                    disabled={isCanceling}
                  >
                    <XCircle className="mr-2 h-4 w-4" />
                    Cancel
                  </Button>
                )}
                {canRetry && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleRetry}
                    disabled={isRetrying}
                    loading={isRetrying}
                  >
                    <RefreshCw className="mr-2 h-4 w-4" />
                    Retry
                  </Button>
                )}
                {canRedeploy && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleRetry}
                    disabled={isRetrying}
                    loading={isRetrying}
                  >
                    <RefreshCw className="mr-2 h-4 w-4" />
                    Redeploy
                  </Button>
                )}
              </div>
            </div>
          </CardHeader>
        </Card>

        {/* Success Banner */}
        {deployment.status === "succeeded" && appUrl && (
          <Card className="border-success bg-success/5">
            <CardContent className="flex flex-col items-center justify-between gap-4 py-6 sm:flex-row">
              <div className="flex items-center gap-3">
                <CheckCircle2 className="h-8 w-8 text-success" />
                <div>
                  <p className="font-medium">Deployment successful!</p>
                  <p className="text-sm text-muted-foreground">
                    Your app is now live at:
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <a
                  href={appUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 rounded-md bg-success px-4 py-2 text-sm font-medium text-success-foreground hover:bg-success/90"
                >
                  <ExternalLink className="h-4 w-4" />
                  Visit App
                </a>
                <Button variant="outline" size="icon" onClick={handleCopyUrl}>
                  {copied ? (
                    <Check className="h-4 w-4 text-success" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Error Banner */}
        {deployment.status === "failed" && deployment.error && (
          <Card className="border-destructive bg-destructive/5">
            <CardContent className="py-6">
              <div className="flex items-start gap-3">
                <XCircle className="h-6 w-6 text-destructive" />
                <div>
                  <p className="font-medium text-destructive">Deployment failed</p>
                  <p className="mt-1 text-sm">{deployment.error}</p>
                </div>
              </div>
            </CardContent>
          </Card>
        )}

        <div className="grid gap-6 lg:grid-cols-2">
          {/* Timeline */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Deployment Steps</CardTitle>
            </CardHeader>
            <CardContent>
              <DeploymentTimeline steps={deployment.steps} />
            </CardContent>
          </Card>

          {/* Logs */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Logs</CardTitle>
            </CardHeader>
            <CardContent>
              <LogViewer deploymentId={deploymentId} />
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Cancel Confirmation Dialog */}
      <Dialog open={showCancelDialog} onOpenChange={setShowCancelDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cancel Deployment</DialogTitle>
            <DialogDescription>
              Are you sure you want to cancel this deployment? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowCancelDialog(false)}>
              Keep Running
            </Button>
            <Button
              variant="destructive"
              onClick={handleCancel}
              disabled={isCanceling}
              loading={isCanceling}
            >
              Cancel Deployment
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
