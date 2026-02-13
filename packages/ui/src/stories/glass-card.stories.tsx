import type { Meta, StoryObj } from "@storybook/react";
import {
  GlassCard,
  GlassCardHeader,
  GlassCardContent,
  GlassCardFooter,
} from "../components/glass-card";
import { Button } from "../components/button";

const meta: Meta<typeof GlassCard> = {
  title: "SpaceScale/GlassCard",
  component: GlassCard,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "elevated", "flat", "interactive"],
    },
    padding: {
      control: "select",
      options: ["none", "sm", "default", "lg"],
    },
  },
  decorators: [
    (Story) => (
      <div className="rounded-xl bg-gradient-to-br from-indigo-500/20 via-purple-500/10 to-pink-500/20 p-12">
        <Story />
      </div>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof GlassCard>;

export const Default: Story = {
  render: () => (
    <GlassCard className="w-[350px]">
      <GlassCardHeader>
        <h3 className="text-lg font-semibold">Deployments</h3>
        <p className="text-sm text-muted-foreground">
          Recent deployment activity
        </p>
      </GlassCardHeader>
      <GlassCardContent>
        <p className="text-sm">3 active deployments running</p>
      </GlassCardContent>
      <GlassCardFooter>
        <Button size="sm">View All</Button>
      </GlassCardFooter>
    </GlassCard>
  ),
};

export const AllVariants: Story = {
  render: () => (
    <div className="grid grid-cols-2 gap-4">
      <GlassCard variant="default" className="p-6">
        <h4 className="font-medium">Default</h4>
        <p className="text-sm text-muted-foreground">Standard glass surface</p>
      </GlassCard>
      <GlassCard variant="elevated" className="p-6">
        <h4 className="font-medium">Elevated</h4>
        <p className="text-sm text-muted-foreground">Hover to see glow</p>
      </GlassCard>
      <GlassCard variant="flat" className="p-6">
        <h4 className="font-medium">Flat</h4>
        <p className="text-sm text-muted-foreground">Subtle background</p>
      </GlassCard>
      <GlassCard variant="interactive" className="p-6">
        <h4 className="font-medium">Interactive</h4>
        <p className="text-sm text-muted-foreground">Clickable card</p>
      </GlassCard>
    </div>
  ),
};

export const DashboardLayout: Story = {
  render: () => (
    <div className="grid grid-cols-3 gap-4 w-[700px]">
      {["CPU", "Memory", "Requests"].map((label) => (
        <GlassCard key={label} variant="elevated" padding="default">
          <span className="text-xs uppercase tracking-wider text-muted-foreground">
            {label}
          </span>
          <p className="mt-1 text-2xl font-semibold">42%</p>
        </GlassCard>
      ))}
    </div>
  ),
};
