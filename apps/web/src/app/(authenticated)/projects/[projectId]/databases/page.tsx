import { Database } from "lucide-react";

export default function DatabasesPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] text-center">
      <div className="mb-6 flex h-14 w-14 items-center justify-center rounded-xl border border-border/60 bg-muted/40">
        <Database className="h-6 w-6 text-muted-foreground" strokeWidth={1.5} />
      </div>
      <h1 className="mb-2 text-xl font-[300] tracking-tight text-foreground">Databases</h1>
      <p className="text-sm text-muted-foreground max-w-xs">
        Managed databases are coming soon. Postgres, Redis, and more — provisioned in seconds.
      </p>
    </div>
  );
}
