import type { Meta, StoryObj } from "@storybook/react";
import { SearchInput } from "../components/search-input";

const meta: Meta<typeof SearchInput> = {
  title: "SpaceScale/SearchInput",
  component: SearchInput,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "glass"],
    },
    showClear: { control: "boolean" },
    disabled: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof SearchInput>;

export const Default: Story = {
  args: { placeholder: "Search projects..." },
};

export const Glass: Story = {
  args: { variant: "glass", placeholder: "Filter logs..." },
  decorators: [
    (Story) => (
      <div className="rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 p-8">
        <div className="w-[320px]">
          <Story />
        </div>
      </div>
    ),
  ],
};

export const WithValue: Story = {
  args: { placeholder: "Search...", defaultValue: "my-app" },
};

export const Disabled: Story = {
  args: { disabled: true, placeholder: "Search disabled" },
};
