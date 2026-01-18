"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Plus, Trash2, Check, X, Loader2, AlertTriangle } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui";
import { useUpdateApp, useDeleteApp, useSubdomainCheck, toast } from "@/lib/hooks";
import { isValidSubdomain, cn } from "@/lib/utils";
import type { App, EnvVar } from "@/lib/schemas";

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

interface SettingsTabProps {
  app: App;
}

export function SettingsTab({ app }: SettingsTabProps) {
  const router = useRouter();
  const [name, setName] = useState(app.name);
  const [subdomain, setSubdomain] = useState(app.subdomain);
  const [envVars, setEnvVars] = useState<EnvVar[]>(app.envVars || []);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");

  const { mutate: updateApp, isPending: isUpdating } = useUpdateApp(app.id);
  const { mutate: deleteApp, isPending: isDeleting } = useDeleteApp(app.id);

  const subdomainChanged = subdomain !== app.subdomain;
  const { data: subdomainCheck, isLoading: checkingSubdomain } = useSubdomainCheck(
    subdomainChanged ? subdomain : ""
  );

  const subdomainValid = subdomain.length > 0 && isValidSubdomain(subdomain);
  const subdomainAvailable = !subdomainChanged || subdomainCheck?.available;

  const hasChanges =
    name !== app.name ||
    subdomain !== app.subdomain ||
    JSON.stringify(envVars) !== JSON.stringify(app.envVars || []);

  const canSave =
    name.length > 0 &&
    subdomainValid &&
    subdomainAvailable &&
    hasChanges;

  const handleAddEnvVar = () => {
    setEnvVars([...envVars, { key: "", value: "" }]);
  };

  const handleUpdateEnvVar = (index: number, field: "key" | "value", value: string) => {
    setEnvVars(
      envVars.map((env, i) =>
        i === index ? { ...env, [field]: field === "key" ? value.toUpperCase() : value } : env
      )
    );
  };

  const handleRemoveEnvVar = (index: number) => {
    setEnvVars(envVars.filter((_, i) => i !== index));
  };

  const handleSave = () => {
    updateApp({
      name: name !== app.name ? name : undefined,
      subdomain: subdomain !== app.subdomain ? subdomain : undefined,
      envVars: JSON.stringify(envVars) !== JSON.stringify(app.envVars || []) ? envVars : undefined,
    });
  };

  const handleDelete = () => {
    deleteApp(undefined, {
      onSuccess: () => {
        router.push("/app");
      },
    });
  };

  return (
    <div className="space-y-6">
      {/* General Settings */}
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">General</CardTitle>
          <CardDescription>Basic app configuration</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="app-name">App Name</Label>
            <Input
              id="app-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={63}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="subdomain">Subdomain</Label>
            <div className="flex items-center gap-2">
              <div className="relative flex-1">
                <Input
                  id="subdomain"
                  value={subdomain}
                  onChange={(e) => setSubdomain(e.target.value.toLowerCase())}
                  maxLength={63}
                  className={cn(
                    "pr-10",
                    subdomainValid && subdomainAvailable && "border-success",
                    subdomainChanged && subdomainValid && subdomainAvailable === false && "border-destructive"
                  )}
                />
                <div className="absolute right-3 top-1/2 -translate-y-1/2">
                  {checkingSubdomain && (
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                  )}
                  {!checkingSubdomain && subdomainValid && subdomainAvailable && (
                    <Check className="h-4 w-4 text-success" />
                  )}
                  {!checkingSubdomain && subdomainChanged && subdomainValid && subdomainAvailable === false && (
                    <X className="h-4 w-4 text-destructive" />
                  )}
                </div>
              </div>
              <span className="text-sm text-muted-foreground">.{PLATFORM_DOMAIN}</span>
            </div>
            {subdomainChanged && subdomainValid && subdomainAvailable === false && subdomainCheck?.suggestion && (
              <p className="text-xs text-muted-foreground">
                Try:{" "}
                <button
                  type="button"
                  className="text-primary underline"
                  onClick={() => setSubdomain(subdomainCheck.suggestion!)}
                >
                  {subdomainCheck.suggestion}
                </button>
              </p>
            )}
          </div>

          <Button onClick={handleSave} disabled={!canSave || isUpdating} loading={isUpdating}>
            Save Changes
          </Button>
        </CardContent>
      </Card>

      {/* Environment Variables */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-lg">Environment Variables</CardTitle>
              <CardDescription>Manage your app&apos;s environment configuration</CardDescription>
            </div>
            <Button variant="outline" size="sm" onClick={handleAddEnvVar}>
              <Plus className="mr-1 h-4 w-4" />
              Add
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {envVars.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No environment variables configured
            </p>
          ) : (
            <div className="space-y-2">
              {envVars.map((env, index) => (
                <div key={index} className="flex items-center gap-2">
                  <Input
                    placeholder="KEY"
                    value={env.key}
                    onChange={(e) => handleUpdateEnvVar(index, "key", e.target.value)}
                    className="flex-1 font-mono text-sm"
                    aria-label={`Environment variable key ${index + 1}`}
                  />
                  <Input
                    placeholder="value"
                    value={env.value}
                    onChange={(e) => handleUpdateEnvVar(index, "value", e.target.value)}
                    className="flex-1 font-mono text-sm"
                    type="password"
                    aria-label={`Environment variable value ${index + 1}`}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleRemoveEnvVar(index)}
                    aria-label={`Remove environment variable ${index + 1}`}
                  >
                    <Trash2 className="h-4 w-4 text-muted-foreground" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          {envVars.length > 0 && (
            <Button
              className="mt-4"
              onClick={handleSave}
              disabled={!hasChanges || isUpdating}
              loading={isUpdating}
            >
              Save Environment Variables
            </Button>
          )}
        </CardContent>
      </Card>

      {/* Danger Zone */}
      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg text-destructive">
            <AlertTriangle className="h-5 w-5" />
            Danger Zone
          </CardTitle>
          <CardDescription>
            Irreversible and destructive actions
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between rounded-lg border border-destructive/30 p-4">
            <div>
              <p className="font-medium">Delete this app</p>
              <p className="text-sm text-muted-foreground">
                Permanently delete this app and all its data
              </p>
            </div>
            <Button variant="destructive" onClick={() => setShowDeleteDialog(true)}>
              Delete App
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete App</DialogTitle>
            <DialogDescription>
              This action cannot be undone. This will permanently delete the app
              <strong> {app.name}</strong> and all associated data.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="delete-confirmation">
              Type <strong>{app.name}</strong> to confirm
            </Label>
            <Input
              id="delete-confirmation"
              value={deleteConfirmation}
              onChange={(e) => setDeleteConfirmation(e.target.value)}
              placeholder={app.name}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteConfirmation !== app.name || isDeleting}
              loading={isDeleting}
            >
              Delete App
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
