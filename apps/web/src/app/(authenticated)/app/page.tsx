"use client";

import { useState } from "react";
import Link from "next/link";
import { Plus, Search, ExternalLink, Copy, Check, Rocket } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Badge,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@/components/ui";
import { PageHeader } from "@/components/layout";
import { useApps, toast } from "@/lib/hooks";
import { formatRelativeTime, copyToClipboard } from "@/lib/utils";
import type { App, AppStatus } from "@/lib/schemas";

const statusLabels: Record<AppStatus, string> = {
  live: "Live",
  deploying: "Deploying",
  failed: "Failed",
  stopped: "Stopped",
};

function AppCard({ app }: { app: App }) {
  const [copied, setCopied] = useState(false);

  const handleCopyUrl = async () => {
    if (app.url) {
      await copyToClipboard(app.url);
      setCopied(true);
      toast({ title: "URL copied to clipboard" });
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <Card className="transition-shadow hover:shadow-md">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <CardTitle className="text-lg">
              <Link
                href={`/app/apps/${app.id}`}
                className="hover:underline"
              >
                {app.name}
              </Link>
            </CardTitle>
            <CardDescription className="text-sm">
              {app.subdomain}.{process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io"}
            </CardDescription>
          </div>
          <Badge variant={app.status}>{statusLabels[app.status]}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {app.url && (
            <div className="flex items-center gap-2">
              <a
                href={app.url}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1 text-sm text-primary hover:underline"
              >
                <ExternalLink className="h-3 w-3" />
                {app.url}
              </a>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={handleCopyUrl}
                aria-label="Copy URL"
              >
                {copied ? (
                  <Check className="h-3 w-3 text-success" />
                ) : (
                  <Copy className="h-3 w-3" />
                )}
              </Button>
            </div>
          )}
          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <span>
              {app.source.type === "github" && `${app.source.repository} (${app.source.branch})`}
              {app.source.type === "dockerfile" && `${app.source.repository}`}
              {app.source.type === "docker_image" && `${app.source.image}:${app.source.tag || "latest"}`}
            </span>
            <span>{formatRelativeTime(app.updatedAt)}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function AppCardSkeleton() {
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="space-y-2">
            <Skeleton className="h-5 w-32" />
            <Skeleton className="h-4 w-48" />
          </div>
          <Skeleton className="h-5 w-16 rounded-full" />
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          <Skeleton className="h-4 w-64" />
          <div className="flex justify-between">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-4 w-16" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function EmptyState() {
  return (
    <Card className="py-12">
      <CardContent className="flex flex-col items-center justify-center text-center">
        <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-muted">
          <Rocket className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="mb-2 text-lg font-semibold">No apps yet</h3>
        <p className="mb-6 max-w-sm text-muted-foreground">
          Get started by deploying your first application. Connect a GitHub repository, Dockerfile,
          or Docker image.
        </p>
        <Button asChild>
          <Link href="/app/new">
            <Plus className="mr-2 h-4 w-4" />
            Deploy New App
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}

export default function DashboardPage() {
  const { data, isLoading, error } = useApps();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("all");

  const filteredApps = data?.apps.filter((app) => {
    const matchesSearch = app.name.toLowerCase().includes(search.toLowerCase()) ||
      app.subdomain.toLowerCase().includes(search.toLowerCase());
    const matchesStatus = statusFilter === "all" || app.status === statusFilter;
    return matchesSearch && matchesStatus;
  });

  return (
    <>
      <PageHeader
        title="Dashboard"
        description="Manage your deployed applications"
      >
        <Button asChild>
          <Link href="/app/new">
            <Plus className="mr-2 h-4 w-4" />
            Deploy New App
          </Link>
        </Button>
      </PageHeader>

      {/* Filters */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search apps..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
            aria-label="Search apps"
          />
        </div>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-full sm:w-40" aria-label="Filter by status">
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="live">Live</SelectItem>
            <SelectItem value="deploying">Deploying</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="stopped">Stopped</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <AppCardSkeleton key={i} />
          ))}
        </div>
      ) : error ? (
        <Card className="py-12">
          <CardContent className="text-center">
            <p className="text-destructive">Failed to load apps. Please try again.</p>
            <Button variant="outline" className="mt-4" onClick={() => window.location.reload()}>
              Retry
            </Button>
          </CardContent>
        </Card>
      ) : !filteredApps || filteredApps.length === 0 ? (
        search || statusFilter !== "all" ? (
          <Card className="py-12">
            <CardContent className="text-center">
              <p className="text-muted-foreground">No apps match your filters.</p>
              <Button
                variant="outline"
                className="mt-4"
                onClick={() => {
                  setSearch("");
                  setStatusFilter("all");
                }}
              >
                Clear filters
              </Button>
            </CardContent>
          </Card>
        ) : (
          <EmptyState />
        )
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {filteredApps.map((app) => (
            <AppCard key={app.id} app={app} />
          ))}
        </div>
      )}
    </>
  );
}
