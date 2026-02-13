import type { Meta, StoryObj } from "@storybook/react";
import { CodeBlock } from "../components/code-block";

const meta: Meta<typeof CodeBlock> = {
  title: "SpaceScale/CodeBlock",
  component: CodeBlock,
  tags: ["autodocs"],
  argTypes: {
    showLineNumbers: { control: "boolean" },
    copyable: { control: "boolean" },
  },
};
export default meta;
type Story = StoryObj<typeof CodeBlock>;

export const Default: Story = {
  args: {
    code: "npm install @spacescale/ui",
    language: "bash",
    title: "Terminal",
  },
};

export const WithLineNumbers: Story = {
  args: {
    title: "next.config.ts",
    language: "typescript",
    showLineNumbers: true,
    code: `import type { NextConfig } from "next";

const config: NextConfig = {
  transpilePackages: ["@spacescale/ui"],
  experimental: {
    turbo: {},
  },
};

export default config;`,
  },
};

export const BuildOutput: Story = {
  args: {
    title: "Build Output",
    code: `[11:32:01] Cloning repository...
[11:32:03] Installing dependencies...
[11:32:15] npm install completed (12s)
[11:32:16] Building application...
[11:32:28] ✓ Compiled successfully
[11:32:28] Build output: 2.4 MB
[11:32:29] Deploying to production...
[11:32:35] ✓ Deployment complete
[11:32:35] URL: https://my-app.spacescale.app`,
  },
};

export const NoCopy: Story = {
  args: {
    code: `export default function Home() {\n  return <h1>Hello World</h1>;\n}`,
    language: "tsx",
    copyable: false,
    showLineNumbers: true,
  },
};
