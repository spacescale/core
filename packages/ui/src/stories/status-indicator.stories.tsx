import type { Meta, StoryObj } from "@storybook/react";
import { StatusIndicator } from "../components/status-indicator";

const meta: Meta<typeof StatusIndicator> = {
  title: "SpaceScale/StatusIndicator",
  component: StatusIndicator,
  tags: ["autodocs"],
  argTypes: {
    status: {
      control: "select",
      options: [
        "live",
        "running",
        "deploying",
        "building",
        "queued",
        "warning",
        "error",
        "failed",
        "stopped",
        "idle",
      ],
    },
    size: {
      control: "select",
      options: ["sm", "default", "lg"],
    },
    label: { control: "text" },
  },
};
export default meta;
type Story = StoryObj<typeof StatusIndicator>;

export const Live: Story = {
  args: { status: "live", label: "Live" },
};

export const AllStatuses: Story = {
  render: () => (
    <div className="flex flex-col gap-3">
      <StatusIndicator status="live" label="Live — serving traffic" />
      <StatusIndicator status="running" label="Running" />
      <StatusIndicator status="deploying" label="Deploying v2.3..." />
      <StatusIndicator status="building" label="Building..." />
      <StatusIndicator status="queued" label="Queued" />
      <StatusIndicator status="warning" label="Warning — high latency" />
      <StatusIndicator status="error" label="Error — unhealthy" />
      <StatusIndicator status="failed" label="Failed" />
      <StatusIndicator status="stopped" label="Stopped" />
      <StatusIndicator status="idle" label="Idle" />
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-6">
      <StatusIndicator status="live" size="sm" label="Small" />
      <StatusIndicator status="live" size="default" label="Default" />
      <StatusIndicator status="live" size="lg" label="Large" />
    </div>
  ),
};

export const WithoutLabel: Story = {
  render: () => (
    <div className="flex items-center gap-4">
      <StatusIndicator status="live" />
      <StatusIndicator status="deploying" />
      <StatusIndicator status="error" />
      <StatusIndicator status="stopped" />
    </div>
  ),
};
