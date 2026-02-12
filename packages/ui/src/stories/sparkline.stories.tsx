import type { Meta, StoryObj } from "@storybook/react";
import { Sparkline } from "../components/sparkline";

const meta: Meta<typeof Sparkline> = {
  title: "SpaceScale/Sparkline",
  component: Sparkline,
  tags: ["autodocs"],
  argTypes: {
    color: {
      control: "select",
      options: ["primary", "green", "red", "amber", "blue", "purple"],
    },
    fill: { control: "boolean" },
    strokeWidth: { control: { type: "range", min: 1, max: 4, step: 0.5 } },
  },
};
export default meta;
type Story = StoryObj<typeof Sparkline>;

const sampleData = [10, 25, 18, 40, 35, 55, 48, 60, 42, 70, 65, 80];

export const Default: Story = {
  args: { data: sampleData },
  decorators: [(Story) => <div className="w-[200px]"><Story /></div>],
};

export const WithFill: Story = {
  args: { data: sampleData, fill: true, color: "blue" },
  decorators: [(Story) => <div className="w-[200px]"><Story /></div>],
};

export const AllColors: Story = {
  render: () => (
    <div className="flex flex-col gap-4 w-[200px]">
      {(["primary", "green", "red", "amber", "blue", "purple"] as const).map(
        (color) => (
          <div key={color} className="flex items-center gap-3">
            <span className="text-xs text-muted-foreground w-14 capitalize">{color}</span>
            <Sparkline data={sampleData} color={color} fill className="flex-1" />
          </div>
        )
      )}
    </div>
  ),
};

export const RealWorldMetric: Story = {
  render: () => (
    <div className="flex items-center gap-4 rounded-lg border bg-card p-4 w-[300px]">
      <div>
        <p className="text-xs text-muted-foreground">CPU Usage</p>
        <p className="text-xl font-semibold">42%</p>
      </div>
      <Sparkline
        data={[20, 35, 28, 42, 38, 55, 48, 42]}
        color="blue"
        fill
        className="flex-1"
      />
    </div>
  ),
};
