"use client";

import { useRouter } from "next/navigation";
import { ArrowLeft, ArrowRight, Rocket } from "lucide-react";
import { Button, Card, CardContent } from "@/components/ui";
import { PageHeader } from "@/components/layout";
import {
  WizardSteps,
  StepSource,
  StepBuild,
  StepConfigure,
  StepPlan,
  StepReview,
} from "@/components/deploy-wizard";
import { useDeployWizardStore, type WizardStep } from "@/stores/deploy-wizard";
import { useCreateDeployment } from "@/lib/hooks";

const stepOrder: WizardStep[] = ["source", "build", "configure", "plan", "review"];

const stepComponents: Record<WizardStep, React.ComponentType> = {
  source: StepSource,
  build: StepBuild,
  configure: StepConfigure,
  plan: StepPlan,
  review: StepReview,
};

export default function DeployWizardPage() {
  const router = useRouter();
  const {
    currentStep,
    setCurrentStep,
    canProceed,
    getSource,
    appName,
    subdomain,
    selectedPlan,
    envVars,
    buildCommand,
    startCommand,
    port,
    reset,
  } = useDeployWizardStore();

  const { mutate: createDeployment, isPending } = useCreateDeployment();

  const currentIndex = stepOrder.indexOf(currentStep);
  const isFirstStep = currentIndex === 0;
  const isLastStep = currentIndex === stepOrder.length - 1;
  const canGoNext = canProceed(currentStep);

  const handleNext = () => {
    if (isLastStep) {
      handleDeploy();
    } else {
      setCurrentStep(stepOrder[currentIndex + 1]);
    }
  };

  const handleBack = () => {
    if (!isFirstStep) {
      setCurrentStep(stepOrder[currentIndex - 1]);
    }
  };

  const handleStepClick = (step: WizardStep) => {
    const targetIndex = stepOrder.indexOf(step);
    // Can only navigate to completed or current steps
    if (targetIndex <= currentIndex) {
      setCurrentStep(step);
    }
  };

  const canNavigateToStep = (step: WizardStep) => {
    const targetIndex = stepOrder.indexOf(step);
    return targetIndex <= currentIndex;
  };

  const handleDeploy = () => {
    const source = getSource();
    if (!source) return;

    createDeployment(
      {
        name: appName,
        subdomain,
        source,
        plan: selectedPlan,
        envVars: envVars.filter((env) => env.key && env.value),
        buildCommand: buildCommand || undefined,
        startCommand: startCommand || undefined,
        port,
      },
      {
        onSuccess: (data) => {
          reset();
          // Navigate to the new deployment
          if (data.deployment?.id) {
            router.push(`/app/deployments/${data.deployment.id}`);
          } else {
            router.push("/app");
          }
        },
      }
    );
  };

  const StepComponent = stepComponents[currentStep];

  return (
    <>
      <PageHeader
        title="Deploy New App"
        description="Configure and deploy your application"
      />

      <Card>
        <CardContent className="pt-6">
          <WizardSteps
            currentStep={currentStep}
            onStepClick={handleStepClick}
            canNavigate={canNavigateToStep}
          />

          <div className="mt-12">
            <StepComponent />
          </div>

          {/* Navigation buttons */}
          <div className="mt-8 flex items-center justify-between border-t pt-6">
            <Button
              variant="outline"
              onClick={handleBack}
              disabled={isFirstStep}
            >
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Button>

            <Button
              onClick={handleNext}
              disabled={!canGoNext || isPending}
              loading={isPending}
            >
              {isLastStep ? (
                <>
                  <Rocket className="mr-2 h-4 w-4" />
                  Deploy
                </>
              ) : (
                <>
                  Next
                  <ArrowRight className="ml-2 h-4 w-4" />
                </>
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </>
  );
}
