"use client";

import { Check } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui";
import { cn } from "@/lib/utils";
import { useDeployWizardStore } from "@/stores/deploy-wizard";
import type { Plan } from "@/lib/schemas";

const plans: { id: Plan; name: string; description: string; features: string[] }[] = [
  {
    id: "starter",
    name: "Starter",
    description: "For small projects and testing",
    features: ["256 MB RAM", "0.25 vCPU", "10 GB bandwidth", "Auto-sleep after inactivity"],
  },
  {
    id: "standard",
    name: "Standard",
    description: "For production workloads",
    features: ["1 GB RAM", "1 vCPU", "100 GB bandwidth", "Always-on", "Custom domains"],
  },
  {
    id: "pro",
    name: "Pro",
    description: "For high-performance applications",
    features: ["4 GB RAM", "2 vCPU", "Unlimited bandwidth", "Priority support", "Auto-scaling"],
  },
];

export function StepPlan() {
  const { selectedPlan, setSelectedPlan } = useDeployWizardStore();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Select a Plan</h2>
        <p className="text-sm text-muted-foreground">
          Choose the resources for your application
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {plans.map((plan) => (
          <Card
            key={plan.id}
            className={cn(
              "cursor-pointer transition-all hover:border-primary",
              selectedPlan === plan.id && "border-2 border-primary"
            )}
            onClick={() => setSelectedPlan(plan.id)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => e.key === "Enter" && setSelectedPlan(plan.id)}
            aria-pressed={selectedPlan === plan.id}
          >
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle className="text-lg">{plan.name}</CardTitle>
                {selectedPlan === plan.id && (
                  <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary">
                    <Check className="h-4 w-4 text-primary-foreground" />
                  </div>
                )}
              </div>
              <CardDescription>{plan.description}</CardDescription>
            </CardHeader>
            <CardContent>
              <ul className="space-y-2">
                {plan.features.map((feature, index) => (
                  <li key={index} className="flex items-center gap-2 text-sm">
                    <Check className="h-4 w-4 text-success" />
                    {feature}
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
