"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, AlertCircle } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Skeleton,
  Badge,
} from "@/components/ui";
import { PageHeader } from "@/components/layout";
import { OverviewTab, DeploymentsTab, LogsTab, SettingsTab } from "@/components/app-detail";
import { useApp } from "@/lib/hooks";

const statusLabels = {
  live: "Live",
  deploying: "Deploying",
  failed: "Failed",
  stopped: "Stopped",
};

export default function AppDetailPage() {
  const params = useParams();
  const appId = params.appId as string;

  const { data: app, isLoading, error } = useApp(appId);

  if (isLoading) {
    return (
      <>
        <PageHeader title="">
          <Skeleton className="h-9 w-32" />
        </PageHeader>
        <div className="space-y-6">
          <div className="flex items-center gap-4">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-6 w-20 rounded-full" />
          </div>
          <Card>
            <CardContent className="pt-6">
              <div className="space-y-4">
                <Skeleton className="h-40 w-full" />
              </div>
            </CardContent>
          </Card>
        </div>
      </>
    );
  }

  if (error || !app) {
    return (
      <>
        <PageHeader title="App Not Found" />
        <Card className="py-12">
          <CardContent className="text-center">
            <AlertCircle className="mx-auto h-12 w-12 text-destructive" />
            <h3 className="mt-4 text-lg font-semibold">App not found</h3>
            <p className="mt-2 text-muted-foreground">
              The app you are looking for does not exist or has been deleted.
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

  return (
    <>
      <PageHeader title={app.name}>
        <Button variant="outline" asChild>
          <Link href="/app">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Dashboard
          </Link>
        </Button>
      </PageHeader>

      <div className="mb-6 flex items-center gap-3">
        <Badge variant={app.status} className="text-sm">
          {statusLabels[app.status]}
        </Badge>
        <span className="text-sm text-muted-foreground">
          {app.subdomain}.{process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io"}
        </span>
      </div>

      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="deployments">Deployments</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <OverviewTab app={app} />
        </TabsContent>

        <TabsContent value="deployments">
          <DeploymentsTab appId={app.id} />
        </TabsContent>

        <TabsContent value="logs">
          <LogsTab appId={app.id} />
        </TabsContent>

        <TabsContent value="settings">
          <SettingsTab app={app} />
        </TabsContent>
      </Tabs>
    </>
  );
}
