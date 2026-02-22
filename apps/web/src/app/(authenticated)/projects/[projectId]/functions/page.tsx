import { Zap } from "lucide-react";

export default function FunctionsPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] text-center">
      <div className="mb-6 flex h-14 w-14 items-center justify-center rounded-xl border border-border/60 bg-muted/40">
        <Zap className="h-6 w-6 text-muted-foreground" strokeWidth={1.5} />
      </div>
      <h1 className="mb-2 text-xl font-[300] tracking-tight text-foreground">Functions</h1>
      <p className="text-sm text-muted-foreground max-w-xs">
        Serverless functions are coming soon. Deploy edge functions to 35+ regions with zero config.
      </p>
    </div>
  );
}
