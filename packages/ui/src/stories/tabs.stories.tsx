import type { Meta, StoryObj } from "@storybook/react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../components/tabs";

const meta: Meta<typeof Tabs> = {
  title: "Core/Tabs",
  component: Tabs,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Tabs>;

export const Default: Story = {
  render: () => (
    <Tabs defaultValue="overview" className="w-[400px]">
      <TabsList>
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="metrics">Metrics</TabsTrigger>
        <TabsTrigger value="logs">Logs</TabsTrigger>
        <TabsTrigger value="settings">Settings</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">
        <p className="text-sm text-muted-foreground">
          Application overview and health status.
        </p>
      </TabsContent>
      <TabsContent value="metrics">
        <p className="text-sm text-muted-foreground">
          CPU, memory, and request metrics.
        </p>
      </TabsContent>
      <TabsContent value="logs">
        <p className="text-sm text-muted-foreground">
          Application logs and output.
        </p>
      </TabsContent>
      <TabsContent value="settings">
        <p className="text-sm text-muted-foreground">
          Environment variables and configuration.
        </p>
      </TabsContent>
    </Tabs>
  ),
};
