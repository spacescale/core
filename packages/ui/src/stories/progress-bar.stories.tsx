import type { Meta, StoryObj } from "@storybook/react";
import { ProgressBar } from "../components/progress-bar";

const meta: Meta<typeof ProgressBar> = {
  title: "SpaceScale/ProgressBar",
  component: ProgressBar,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "success", "warning", "destructive", "gradient", "glass"],
    },
    size: {
      control: "select",
      options: ["sm", "default", "lg", "xl"],
    },
    value: { control: { type: "range", min: 0, max: 100 } },
    showValue: { control: "boolean" },
    animated: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof ProgressBar>;

export const Default: Story = {
  args: { value: 65 },
};

export const WithLabel: Story = {
  args: { value: 65, label: "Deploying...", showValue: true },
};

export const Variants: Story = {
  render: () => (
    <div className="flex w-[400px] flex-col gap-4">
      <ProgressBar value={80} variant="default" label="Default" showValue />
      <ProgressBar value={100} variant="success" label="Success" showValue />
      <ProgressBar value={50} variant="warning" label="Warning" showValue />
      <ProgressBar value={25} variant="destructive" label="Error" showValue />
      <ProgressBar value={65} variant="gradient" label="Gradient" showValue />
      <ProgressBar value={45} variant="glass" label="Glass" showValue />
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex w-[400px] flex-col gap-4">
      <ProgressBar value={60} size="sm" label="Small" />
      <ProgressBar value={60} size="default" label="Default" />
      <ProgressBar value={60} size="lg" label="Large" />
      <ProgressBar value={60} size="xl" label="Extra Large" />
    </div>
  ),
};
