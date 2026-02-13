import type { Meta, StoryObj } from "@storybook/react";
import { Switch } from "../components/switch";
import { Label } from "../components/label";

const meta: Meta<typeof Switch> = {
  title: "SpaceScale/Switch",
  component: Switch,
  tags: ["autodocs"],
  argTypes: {
    size: {
      control: "select",
      options: ["sm", "default", "lg"],
    },
    disabled: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof Switch>;

export const Default: Story = {
  args: {},
};

export const WithLabel: Story = {
  render: () => (
    <div className="flex items-center space-x-2">
      <Switch id="auto-deploy" />
      <Label htmlFor="auto-deploy">Auto-deploy on push</Label>
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-6">
      <div className="flex items-center gap-2">
        <Switch size="sm" id="sm" />
        <Label htmlFor="sm">Small</Label>
      </div>
      <div className="flex items-center gap-2">
        <Switch size="default" id="md" />
        <Label htmlFor="md">Default</Label>
      </div>
      <div className="flex items-center gap-2">
        <Switch size="lg" id="lg" />
        <Label htmlFor="lg">Large</Label>
      </div>
    </div>
  ),
};

export const SettingsExample: Story = {
  render: () => (
    <div className="w-[400px] space-y-4 rounded-lg border p-6">
      <h3 className="font-semibold">Deploy Settings</h3>
      <div className="space-y-3">
        {[
          { id: "auto-deploy", label: "Auto-deploy", desc: "Deploy on every push to main" },
          { id: "preview", label: "Preview deploys", desc: "Create preview for PRs" },
          { id: "public", label: "Public access", desc: "Allow unauthenticated access" },
        ].map((item) => (
          <div key={item.id} className="flex items-center justify-between">
            <div>
              <Label htmlFor={item.id}>{item.label}</Label>
              <p className="text-xs text-muted-foreground">{item.desc}</p>
            </div>
            <Switch id={item.id} />
          </div>
        ))}
      </div>
    </div>
  ),
};
