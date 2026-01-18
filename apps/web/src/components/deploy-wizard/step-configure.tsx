"use client";

import { Plus, Trash2, Check, X, Loader2 } from "lucide-react";
import { Button, Input, Label } from "@/components/ui";
import { useDeployWizardStore } from "@/stores/deploy-wizard";
import { useSubdomainCheck } from "@/lib/hooks";
import { slugify, isValidSubdomain } from "@/lib/utils";
import { cn } from "@/lib/utils";

const PLATFORM_DOMAIN = process.env.NEXT_PUBLIC_PLATFORM_DOMAIN || "ourplatform.io";

export function StepConfigure() {
  const {
    appName,
    setAppName,
    subdomain,
    setSubdomain,
    envVars,
    addEnvVar,
    updateEnvVar,
    removeEnvVar,
  } = useDeployWizardStore();

  const { data: subdomainCheck, isLoading: checkingSubdomain } = useSubdomainCheck(subdomain);

  const handleAppNameChange = (name: string) => {
    setAppName(name);
    // Auto-generate subdomain from app name if subdomain is empty or matches previous auto-generated value
    if (!subdomain || subdomain === slugify(appName)) {
      setSubdomain(slugify(name));
    }
  };

  const subdomainValid = subdomain.length > 0 && isValidSubdomain(subdomain);
  const subdomainAvailable = subdomainCheck?.available;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Configure</h2>
        <p className="text-sm text-muted-foreground">
          Set up your app name, subdomain, and environment variables
        </p>
      </div>

      <div className="space-y-4 rounded-lg border p-4">
        {/* App Name */}
        <div className="space-y-2">
          <Label htmlFor="app-name">App Name</Label>
          <Input
            id="app-name"
            placeholder="My Awesome App"
            value={appName}
            onChange={(e) => handleAppNameChange(e.target.value)}
            maxLength={63}
          />
        </div>

        {/* Subdomain */}
        <div className="space-y-2">
          <Label htmlFor="subdomain">Subdomain</Label>
          <div className="flex items-center gap-2">
            <div className="relative flex-1">
              <Input
                id="subdomain"
                placeholder="my-app"
                value={subdomain}
                onChange={(e) => setSubdomain(e.target.value.toLowerCase())}
                maxLength={63}
                className={cn(
                  "pr-10",
                  subdomainValid && subdomainAvailable === true && "border-success",
                  subdomainValid && subdomainAvailable === false && "border-destructive"
                )}
              />
              <div className="absolute right-3 top-1/2 -translate-y-1/2">
                {checkingSubdomain && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
                {!checkingSubdomain && subdomainValid && subdomainAvailable === true && (
                  <Check className="h-4 w-4 text-success" />
                )}
                {!checkingSubdomain && subdomainValid && subdomainAvailable === false && (
                  <X className="h-4 w-4 text-destructive" />
                )}
              </div>
            </div>
            <span className="text-sm text-muted-foreground">.{PLATFORM_DOMAIN}</span>
          </div>
          {subdomainValid && subdomainAvailable === false && subdomainCheck?.suggestion && (
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
          {!subdomainValid && subdomain.length > 0 && (
            <p className="text-xs text-destructive">
              Subdomain must be lowercase alphanumeric with hyphens only
            </p>
          )}
        </div>
      </div>

      {/* Environment Variables */}
      <div className="space-y-4 rounded-lg border p-4">
        <div className="flex items-center justify-between">
          <div>
            <Label>Environment Variables</Label>
            <p className="text-xs text-muted-foreground">
              Add key-value pairs for your app configuration
            </p>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={addEnvVar}>
            <Plus className="mr-1 h-4 w-4" />
            Add
          </Button>
        </div>

        {envVars.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">
            No environment variables added
          </p>
        ) : (
          <div className="space-y-2">
            {envVars.map((env, index) => (
              <div key={index} className="flex items-center gap-2">
                <Input
                  placeholder="KEY"
                  value={env.key}
                  onChange={(e) => updateEnvVar(index, "key", e.target.value.toUpperCase())}
                  className="flex-1 font-mono text-sm"
                  aria-label={`Environment variable key ${index + 1}`}
                />
                <Input
                  placeholder="value"
                  value={env.value}
                  onChange={(e) => updateEnvVar(index, "value", e.target.value)}
                  className="flex-1 font-mono text-sm"
                  aria-label={`Environment variable value ${index + 1}`}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => removeEnvVar(index)}
                  aria-label={`Remove environment variable ${index + 1}`}
                >
                  <Trash2 className="h-4 w-4 text-muted-foreground" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
