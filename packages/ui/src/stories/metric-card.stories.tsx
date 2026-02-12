import type { Meta, StoryObj } from "@storybook/react";
import { Activity, Cpu, HardDrive, Zap } from "lucide-react";
import { MetricCard } from "../components/metric-card";
import { Sparkline } from "../components/sparkline";

const meta: Meta<typeof MetricCard> = {
  title: "SpaceScale/MetricCard",
  component: MetricCard,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "glass", "flat"],
    },
  },
};
export default meta;
type Story = StoryObj<typeof MetricCard>;

export const Default: Story = {
  args: {
    label: "CPU Usage",
    value: "42%",
    trend: { value: 5, direction: "up" },
    icon: <Cpu className="h-4 w-4" />,
  },
};

export const AllTrends: Story = {
  render: () => (
    <div className="grid grid-cols-3 gap-4 w-[600px]">
      <MetricCard
        label="Requests"
        value="1.2M"
        trend={{ value: 12, direction: "up" }}
        icon={<Zap className="h-4 w-4" />}
      />
      <MetricCard
        label="Error Rate"
        value="0.03%"
        trend={{ value: 2, direction: "down" }}
        icon={<Activity className="h-4 w-4" />}
      />
      <MetricCard
        label="Disk"
        value="8.2 GB"
        trend={{ value: 0, direction: "flat" }}
        icon={<HardDrive className="h-4 w-4" />}
      />
    </div>
  ),
};

export const GlassVariant: Story = {
  args: {
    variant: "glass",
    label: "Memory",
    value: "1.2 GB",
    description: "of 4 GB allocated",
    trend: { value: 8, direction: "up" },
  },
  decorators: [
    (Story) => (
      <div className="rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 p-8">
        <div className="w-[250px]">
          <Story />
        </div>
      </div>
    ),
  ],
};

export const WithSparkline: Story = {
  render: () => (
    <div className="w-[250px]">
      <MetricCard
        label="CPU"
        value="42%"
        trend={{ value: 5, direction: "up" }}
        footer={
          <Sparkline
            data={[20, 35, 28, 42, 38, 55, 48, 42]}
            color="blue"
            fill
          />
        }
      />
    </div>
  ),
};
