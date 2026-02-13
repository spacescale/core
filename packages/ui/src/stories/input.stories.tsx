import type { Meta, StoryObj } from "@storybook/react";
import { Input } from "../components/input";
import { Label } from "../components/label";

const meta: Meta<typeof Input> = {
  title: "Core/Input",
  component: Input,
  tags: ["autodocs"],
  argTypes: {
    error: { control: "boolean" },
    disabled: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof Input>;

export const Default: Story = {
  args: { placeholder: "Enter your email..." },
};

export const WithLabel: Story = {
  render: () => (
    <div className="grid w-full max-w-sm gap-1.5">
      <Label htmlFor="email">Email</Label>
      <Input type="email" id="email" placeholder="you@example.com" />
    </div>
  ),
};

export const Error: Story = {
  args: { error: true, placeholder: "Invalid input", defaultValue: "bad@" },
};

export const Disabled: Story = {
  args: { disabled: true, placeholder: "Disabled input" },
};

export const File: Story = {
  args: { type: "file" },
};
