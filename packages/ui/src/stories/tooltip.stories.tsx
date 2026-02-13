import type { Meta, StoryObj } from "@storybook/react";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "../components/tooltip";
import { Button } from "../components/button";
import { Plus } from "lucide-react";

const meta: Meta<typeof Tooltip> = {
  title: "Core/Tooltip",
  component: Tooltip,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Tooltip>;

export const Default: Story = {
  render: () => (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline" size="icon">
            <Plus className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <p>Create new project</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ),
};
