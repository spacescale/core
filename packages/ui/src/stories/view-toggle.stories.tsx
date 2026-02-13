import type { Meta, StoryObj } from "@storybook/react";
import { useState } from "react";
import { ViewToggle } from "../components/view-toggle";

const meta: Meta<typeof ViewToggle> = {
  title: "SpaceScale/ViewToggle",
  component: ViewToggle,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof ViewToggle>;

export const Default: Story = {
  render: () => {
    const [view, setView] = useState<"grid" | "list">("grid");
    return <ViewToggle value={view} onValueChange={setView} />;
  },
};

export const ListView: Story = {
  render: () => {
    const [view, setView] = useState<"grid" | "list">("list");
    return <ViewToggle value={view} onValueChange={setView} />;
  },
};
