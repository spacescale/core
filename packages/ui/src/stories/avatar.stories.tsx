import type { Meta, StoryObj } from "@storybook/react";
import { Avatar, AvatarImage, AvatarFallback } from "../components/avatar";

const meta: Meta<typeof Avatar> = {
  title: "SpaceScale/Avatar",
  component: Avatar,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Avatar>;

export const WithImage: Story = {
  render: () => (
    <Avatar>
      <AvatarImage
        src="https://github.com/shadcn.png"
        alt="User avatar"
      />
      <AvatarFallback>CN</AvatarFallback>
    </Avatar>
  ),
};

export const Fallback: Story = {
  render: () => (
    <Avatar>
      <AvatarFallback>JD</AvatarFallback>
    </Avatar>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      <Avatar className="h-6 w-6">
        <AvatarFallback className="text-[10px]">S</AvatarFallback>
      </Avatar>
      <Avatar className="h-8 w-8">
        <AvatarFallback className="text-xs">M</AvatarFallback>
      </Avatar>
      <Avatar>
        <AvatarFallback>D</AvatarFallback>
      </Avatar>
      <Avatar className="h-14 w-14">
        <AvatarFallback className="text-lg">L</AvatarFallback>
      </Avatar>
    </div>
  ),
};

export const Group: Story = {
  render: () => (
    <div className="flex -space-x-3">
      {["AB", "CD", "EF", "GH"].map((initials) => (
        <Avatar
          key={initials}
          className="border-2 border-background"
        >
          <AvatarFallback>{initials}</AvatarFallback>
        </Avatar>
      ))}
    </div>
  ),
};
