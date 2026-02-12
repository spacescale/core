import type { Meta, StoryObj } from "@storybook/react";
import { Rocket, Search, FileText } from "lucide-react";
import { EmptyState } from "../components/empty-state";
import { Button } from "../components/button";

const meta: Meta<typeof EmptyState> = {
  title: "SpaceScale/EmptyState",
  component: EmptyState,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof EmptyState>;

export const Default: Story = {
  args: {
    icon: <Rocket className="h-12 w-12" />,
    title: "No deployments yet",
    description:
      "Deploy your first application to get started with SpaceScale.",
    action: <Button>Deploy Now</Button>,
  },
};

export const NoResults: Story = {
  args: {
    icon: <Search className="h-10 w-10" />,
    title: "No results found",
    description: 'Try adjusting your search or filter to find what you\'re looking for.',
  },
};

export const NoLogs: Story = {
  args: {
    icon: <FileText className="h-10 w-10" />,
    title: "No logs available",
    description: "Logs will appear here once your application starts running.",
  },
};
