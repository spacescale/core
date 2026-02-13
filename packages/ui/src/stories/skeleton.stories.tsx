import type { Meta, StoryObj } from "@storybook/react";
import { Skeleton } from "../components/skeleton";

const meta: Meta<typeof Skeleton> = {
  title: "Core/Skeleton",
  component: Skeleton,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Skeleton>;

export const Default: Story = {
  render: () => <Skeleton className="h-4 w-[250px]" />,
};

export const CardSkeleton: Story = {
  render: () => (
    <div className="flex w-[350px] items-center space-x-4 rounded-lg border p-4">
      <Skeleton className="h-12 w-12 rounded-full" />
      <div className="space-y-2 flex-1">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    </div>
  ),
};

export const MetricSkeleton: Story = {
  render: () => (
    <div className="grid grid-cols-3 gap-4 w-[600px]">
      {[1, 2, 3].map((i) => (
        <div key={i} className="rounded-xl border p-4 space-y-3">
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-7 w-20" />
          <Skeleton className="h-8 w-full" />
        </div>
      ))}
    </div>
  ),
};
