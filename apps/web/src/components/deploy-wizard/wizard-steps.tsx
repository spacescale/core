"use client";

import { cn } from "@/lib/utils";
import { Check } from "lucide-react";
import type { WizardStep } from "@/stores/deploy-wizard";

const steps: { id: WizardStep; label: string }[] = [
  { id: "source", label: "Source" },
  { id: "build", label: "Build" },
  { id: "configure", label: "Configure" },
  { id: "plan", label: "Plan" },
  { id: "review", label: "Review" },
];

const stepOrder: WizardStep[] = ["source", "build", "configure", "plan", "review"];

interface WizardStepsProps {
  currentStep: WizardStep;
  onStepClick: (step: WizardStep) => void;
  canNavigate: (step: WizardStep) => boolean;
}

export function WizardSteps({ currentStep, onStepClick, canNavigate }: WizardStepsProps) {
  const currentIndex = stepOrder.indexOf(currentStep);

  return (
    <nav aria-label="Progress" className="mb-8">
      <ol className="flex items-center">
        {steps.map((step, index) => {
          const isCompleted = index < currentIndex;
          const isCurrent = step.id === currentStep;
          const canClick = canNavigate(step.id) && index <= currentIndex;

          return (
            <li key={step.id} className={cn("relative", index !== steps.length - 1 && "flex-1")}>
              <div className="flex items-center">
                <button
                  type="button"
                  onClick={() => canClick && onStepClick(step.id)}
                  disabled={!canClick}
                  className={cn(
                    "flex h-10 w-10 items-center justify-center rounded-full text-sm font-medium transition-colors",
                    isCompleted && "bg-primary text-primary-foreground",
                    isCurrent && "border-2 border-primary bg-background text-primary",
                    !isCompleted && !isCurrent && "border-2 border-muted bg-background text-muted-foreground",
                    canClick && "cursor-pointer hover:bg-accent",
                    !canClick && "cursor-default"
                  )}
                  aria-current={isCurrent ? "step" : undefined}
                >
                  {isCompleted ? <Check className="h-5 w-5" /> : index + 1}
                </button>
                {index !== steps.length - 1 && (
                  <div
                    className={cn(
                      "ml-4 h-0.5 flex-1",
                      index < currentIndex ? "bg-primary" : "bg-muted"
                    )}
                  />
                )}
              </div>
              <span
                className={cn(
                  "absolute -bottom-6 left-0 w-max text-xs",
                  isCurrent ? "font-medium text-foreground" : "text-muted-foreground"
                )}
              >
                {step.label}
              </span>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
