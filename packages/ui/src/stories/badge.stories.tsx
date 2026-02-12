import type { Meta, StoryObj } from "@storybook/react";
import { Badge } from "../components/badge";

const meta: Meta<typeof Badge> = {
  title: "Core/Badge",
  component: Badge,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "secondary",
        "destructive",
        "outline",
        "success",
        "warning",
        "live",
        "deploying",
        "failed",
        "stopped",
        "queued",
        "running",
        "succeeded",
        "canceled",
      ],
    },
  },
};
export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = {
  args: { children: "Badge" },
};

export const SemanticVariants: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="default">Default</Badge>
      <Badge variant="secondary">Secondary</Badge>
      <Badge variant="outline">Outline</Badge>
      <Badge variant="success">Success</Badge>
      <Badge variant="warning">Warning</Badge>
      <Badge variant="destructive">Destructive</Badge>
    </div>
  ),
};

export const StatusVariants: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant="live">Live</Badge>
      <Badge variant="running">Running</Badge>
      <Badge variant="deploying">Deploying</Badge>
      <Badge variant="queued">Queued</Badge>
      <Badge variant="succeeded">Succeeded</Badge>
      <Badge variant="failed">Failed</Badge>
      <Badge variant="stopped">Stopped</Badge>
      <Badge variant="canceled">Canceled</Badge>
    </div>
  ),
};
