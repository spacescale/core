import type { Meta, StoryObj } from "@storybook/react";
import { Textarea } from "../components/textarea";
import { Label } from "../components/label";

const meta: Meta<typeof Textarea> = {
  title: "Core/Textarea",
  component: Textarea,
  tags: ["autodocs"],
  argTypes: {
    error: { control: "boolean" },
    disabled: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof Textarea>;

export const Default: Story = {
  args: { placeholder: "Enter your message..." },
};

export const WithLabel: Story = {
  render: () => (
    <div className="grid w-full max-w-sm gap-1.5">
      <Label htmlFor="env">Environment Variables</Label>
      <Textarea id="env" placeholder="KEY=value" />
    </div>
  ),
};

export const Error: Story = {
  args: { error: true, defaultValue: "Invalid YAML" },
};
