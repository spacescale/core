"use client";

import { Input, Label } from "@/components/ui";
import { useDeployWizardStore } from "@/stores/deploy-wizard";

export function StepBuild() {
  const {
    buildCommand,
    setBuildCommand,
    startCommand,
    setStartCommand,
    port,
    setPort,
    sourceType,
  } = useDeployWizardStore();

  const showBuildOptions = sourceType === "github" || sourceType === "dockerfile";

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Build & Runtime</h2>
        <p className="text-sm text-muted-foreground">
          Configure how your application is built and started
        </p>
      </div>

      <div className="space-y-4 rounded-lg border p-4">
        {showBuildOptions && (
          <>
            <div className="space-y-2">
              <Label htmlFor="build-command">Build Command (optional)</Label>
              <Input
                id="build-command"
                placeholder="npm run build"
                value={buildCommand}
                onChange={(e) => setBuildCommand(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Command to build your application. Leave empty if not needed.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="start-command">Start Command (optional)</Label>
              <Input
                id="start-command"
                placeholder="npm start"
                value={startCommand}
                onChange={(e) => setStartCommand(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Command to start your application.
              </p>
            </div>
          </>
        )}

        <div className="space-y-2">
          <Label htmlFor="port">Port</Label>
          <Input
            id="port"
            type="number"
            min={1}
            max={65535}
            placeholder="3000"
            value={port}
            onChange={(e) => setPort(parseInt(e.target.value) || 3000)}
          />
          <p className="text-xs text-muted-foreground">
            The port your application listens on (default: 3000)
          </p>
        </div>
      </div>
    </div>
  );
}
