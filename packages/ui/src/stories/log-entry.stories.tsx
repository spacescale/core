import type { Meta, StoryObj } from "@storybook/react";
import { LogEntry } from "../components/log-entry";

const meta: Meta<typeof LogEntry> = {
  title: "SpaceScale/LogEntry",
  component: LogEntry,
  tags: ["autodocs"],
  argTypes: {
    level: {
      control: "select",
      options: ["info", "warn", "error", "debug", "trace"],
    },
  },
};
export default meta;
type Story = StoryObj<typeof LogEntry>;

export const Info: Story = {
  args: {
    timestamp: "2024-01-15T10:30:00.123Z",
    level: "info",
    message: "Server started on port 8080",
  },
};

export const AllLevels: Story = {
  render: () => (
    <div className="w-[700px] rounded-lg border bg-background p-2 font-mono">
      <LogEntry
        timestamp="2024-01-15T10:30:00.123Z"
        level="info"
        message="Application starting..."
      />
      <LogEntry
        timestamp="2024-01-15T10:30:00.456Z"
        level="debug"
        message="Loading configuration from /etc/app/config.yaml"
        source="config"
      />
      <LogEntry
        timestamp="2024-01-15T10:30:01.789Z"
        level="info"
        message="Connected to database: postgresql://db:5432/app"
      />
      <LogEntry
        timestamp="2024-01-15T10:30:02.012Z"
        level="warn"
        message="Deprecated API endpoint /v1/users still in use"
        source="router"
      />
      <LogEntry
        timestamp="2024-01-15T10:30:02.345Z"
        level="error"
        message="Failed to connect to Redis: ECONNREFUSED 127.0.0.1:6379"
        source="cache"
      />
      <LogEntry
        timestamp="2024-01-15T10:30:02.678Z"
        level="trace"
        message="HTTP GET /health 200 2ms"
      />
      <LogEntry
        timestamp="2024-01-15T10:30:03.001Z"
        level="info"
        message="Server listening on https://0.0.0.0:8080"
      />
    </div>
  ),
};

export const WithSource: Story = {
  args: {
    timestamp: "2024-01-15T10:30:00Z",
    level: "error",
    message: "Connection refused: ECONNREFUSED 127.0.0.1:5432",
    source: "database",
  },
};
