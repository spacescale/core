import type { Meta, StoryObj } from "@storybook/react";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SelectGroup,
  SelectLabel,
} from "../components/select";

const meta: Meta<typeof Select> = {
  title: "Core/Select",
  component: Select,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Select>;

export const Default: Story = {
  render: () => (
    <Select>
      <SelectTrigger className="w-[200px]">
        <SelectValue placeholder="Select region" />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>North America</SelectLabel>
          <SelectItem value="us-east-1">US East (Virginia)</SelectItem>
          <SelectItem value="us-west-2">US West (Oregon)</SelectItem>
        </SelectGroup>
        <SelectGroup>
          <SelectLabel>Europe</SelectLabel>
          <SelectItem value="eu-west-1">EU West (Ireland)</SelectItem>
          <SelectItem value="eu-central-1">EU Central (Frankfurt)</SelectItem>
        </SelectGroup>
      </SelectContent>
    </Select>
  ),
};
