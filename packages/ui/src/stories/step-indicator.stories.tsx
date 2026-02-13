import type { Meta, StoryObj } from "@storybook/react";
import { StepIndicator } from "../components/step-indicator";

const meta: Meta<typeof StepIndicator> = {
  title: "SpaceScale/StepIndicator",
  component: StepIndicator,
  tags: ["autodocs"],
  argTypes: {
    orientation: {
      control: "select",
      options: ["horizontal", "vertical"],
    },
  },
};
export default meta;
type Story = StoryObj<typeof StepIndicator>;

export const DeploymentFlow: Story = {
  args: {
    steps: [
      { label: "Clone", status: "completed", description: "Repository cloned" },
      { label: "Install", status: "completed", description: "Dependencies installed" },
      { label: "Build", status: "active", description: "Compiling assets..." },
      { label: "Deploy", status: "pending", description: "Waiting for build" },
    ],
  },
  decorators: [
    (Story) => (
      <div className="w-[600px]">
        <Story />
      </div>
    ),
  ],
};

export const AllCompleted: Story = {
  args: {
    steps: [
      { label: "Clone", status: "completed" },
      { label: "Install", status: "completed" },
      { label: "Build", status: "completed" },
      { label: "Deploy", status: "completed" },
    ],
  },
  decorators: [
    (Story) => (
      <div className="w-[600px]">
        <Story />
      </div>
    ),
  ],
};

export const WithError: Story = {
  args: {
    steps: [
      { label: "Clone", status: "completed" },
      { label: "Install", status: "completed" },
      { label: "Build", status: "error", description: "Exit code 1" },
      { label: "Deploy", status: "pending" },
    ],
  },
  decorators: [
    (Story) => (
      <div className="w-[600px]">
        <Story />
      </div>
    ),
  ],
};

export const Vertical: Story = {
  args: {
    orientation: "vertical",
    steps: [
      { label: "Clone repository", status: "completed", description: "Cloned from GitHub" },
      { label: "Install dependencies", status: "completed", description: "npm install completed" },
      { label: "Build application", status: "active", description: "Compiling..." },
      { label: "Deploy to production", status: "pending" },
    ],
  },
};
