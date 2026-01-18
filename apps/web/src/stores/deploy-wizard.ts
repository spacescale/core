"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AppSource, EnvVar, Plan } from "@/lib/schemas";

export type WizardStep = "source" | "build" | "configure" | "plan" | "review";

interface DeployWizardState {
  currentStep: WizardStep;

  // Step 1: Source
  sourceType: "github" | "dockerfile" | "docker_image" | null;
  repository: string;
  branch: string;
  dockerfilePath: string;
  dockerImage: string;
  dockerTag: string;

  // Step 2: Build & Runtime
  buildCommand: string;
  startCommand: string;
  port: number;

  // Step 3: Configure
  appName: string;
  subdomain: string;
  envVars: EnvVar[];

  // Step 4: Plan
  selectedPlan: Plan;

  // Actions
  setCurrentStep: (step: WizardStep) => void;
  setSourceType: (type: "github" | "dockerfile" | "docker_image" | null) => void;
  setRepository: (repo: string) => void;
  setBranch: (branch: string) => void;
  setDockerfilePath: (path: string) => void;
  setDockerImage: (image: string) => void;
  setDockerTag: (tag: string) => void;
  setBuildCommand: (cmd: string) => void;
  setStartCommand: (cmd: string) => void;
  setPort: (port: number) => void;
  setAppName: (name: string) => void;
  setSubdomain: (subdomain: string) => void;
  setEnvVars: (vars: EnvVar[]) => void;
  addEnvVar: () => void;
  updateEnvVar: (index: number, field: "key" | "value", value: string) => void;
  removeEnvVar: (index: number) => void;
  setSelectedPlan: (plan: Plan) => void;
  getSource: () => AppSource | null;
  reset: () => void;
  canProceed: (step: WizardStep) => boolean;
}

const initialState = {
  currentStep: "source" as WizardStep,
  sourceType: null,
  repository: "",
  branch: "main",
  dockerfilePath: "Dockerfile",
  dockerImage: "",
  dockerTag: "latest",
  buildCommand: "",
  startCommand: "",
  port: 3000,
  appName: "",
  subdomain: "",
  envVars: [] as EnvVar[],
  selectedPlan: "starter" as Plan,
};

export const useDeployWizardStore = create<DeployWizardState>()(
  persist(
    (set, get) => ({
      ...initialState,

      setCurrentStep: (step) => set({ currentStep: step }),
      setSourceType: (type) => set({ sourceType: type }),
      setRepository: (repo) => set({ repository: repo }),
      setBranch: (branch) => set({ branch }),
      setDockerfilePath: (path) => set({ dockerfilePath: path }),
      setDockerImage: (image) => set({ dockerImage: image }),
      setDockerTag: (tag) => set({ dockerTag: tag }),
      setBuildCommand: (cmd) => set({ buildCommand: cmd }),
      setStartCommand: (cmd) => set({ startCommand: cmd }),
      setPort: (port) => set({ port }),
      setAppName: (name) => set({ appName: name }),
      setSubdomain: (subdomain) => set({ subdomain }),
      setEnvVars: (vars) => set({ envVars: vars }),

      addEnvVar: () =>
        set((state) => ({
          envVars: [...state.envVars, { key: "", value: "" }],
        })),

      updateEnvVar: (index, field, value) =>
        set((state) => ({
          envVars: state.envVars.map((env, i) =>
            i === index ? { ...env, [field]: value } : env
          ),
        })),

      removeEnvVar: (index) =>
        set((state) => ({
          envVars: state.envVars.filter((_, i) => i !== index),
        })),

      setSelectedPlan: (plan) => set({ selectedPlan: plan }),

      getSource: () => {
        const state = get();
        switch (state.sourceType) {
          case "github":
            return {
              type: "github" as const,
              repository: state.repository,
              branch: state.branch,
            };
          case "dockerfile":
            return {
              type: "dockerfile" as const,
              repository: state.repository,
              branch: state.branch,
              dockerfilePath: state.dockerfilePath,
            };
          case "docker_image":
            return {
              type: "docker_image" as const,
              image: state.dockerImage,
              tag: state.dockerTag,
            };
          default:
            return null;
        }
      },

      reset: () => set(initialState),

      canProceed: (step) => {
        const state = get();
        switch (step) {
          case "source":
            if (state.sourceType === "github" || state.sourceType === "dockerfile") {
              return state.repository.length > 0 && state.branch.length > 0;
            }
            if (state.sourceType === "docker_image") {
              return state.dockerImage.length > 0;
            }
            return false;
          case "build":
            return state.port > 0 && state.port < 65536;
          case "configure":
            return (
              state.appName.length > 0 &&
              state.subdomain.length > 0 &&
              state.subdomain.length <= 63
            );
          case "plan":
            return ["starter", "standard", "pro"].includes(state.selectedPlan);
          case "review":
            return true;
          default:
            return false;
        }
      },
    }),
    {
      name: "deploy-wizard-draft",
      partialize: (state) => ({
        currentStep: state.currentStep,
        sourceType: state.sourceType,
        repository: state.repository,
        branch: state.branch,
        dockerfilePath: state.dockerfilePath,
        dockerImage: state.dockerImage,
        dockerTag: state.dockerTag,
        buildCommand: state.buildCommand,
        startCommand: state.startCommand,
        port: state.port,
        appName: state.appName,
        subdomain: state.subdomain,
        envVars: state.envVars,
        selectedPlan: state.selectedPlan,
      }),
    }
  )
);
