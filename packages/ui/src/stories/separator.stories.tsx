import type { Meta, StoryObj } from "@storybook/react";
import { Separator } from "../components/separator";

const meta: Meta<typeof Separator> = {
  title: "SpaceScale/Separator",
  component: Separator,
  tags: ["autodocs"],
  argTypes: {
    orientation: {
      control: "select",
      options: ["horizontal", "vertical"],
    },
  },
};
export default meta;
type Story = StoryObj<typeof Separator>;

export const Horizontal: Story = {
  render: () => (
    <div className="w-[300px] space-y-4">
      <div>
        <h4 className="text-sm font-medium">General</h4>
        <p className="text-sm text-muted-foreground">Basic project settings</p>
      </div>
      <Separator />
      <div>
        <h4 className="text-sm font-medium">Danger Zone</h4>
        <p className="text-sm text-muted-foreground">Irreversible actions</p>
      </div>
    </div>
  ),
};

export const Vertical: Story = {
  render: () => (
    <div className="flex h-5 items-center space-x-4 text-sm">
      <span>Docs</span>
      <Separator orientation="vertical" />
      <span>API</span>
      <Separator orientation="vertical" />
      <span>Status</span>
    </div>
  ),
};
