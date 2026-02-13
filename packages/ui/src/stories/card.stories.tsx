import type { Meta, StoryObj } from "@storybook/react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "../components/card";
import { Button } from "../components/button";

const meta: Meta<typeof Card> = {
  title: "Core/Card",
  component: Card,
  tags: ["autodocs"],
};
export default meta;
type Story = StoryObj<typeof Card>;

export const Default: Story = {
  render: () => (
    <Card className="w-[350px]">
      <CardHeader>
        <CardTitle>Project Settings</CardTitle>
        <CardDescription>
          Manage your project configuration and environment.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">
          Your project is currently running on the Pro plan with 3 active
          deployments.
        </p>
      </CardContent>
      <CardFooter className="justify-end gap-2">
        <Button variant="outline">Cancel</Button>
        <Button>Save</Button>
      </CardFooter>
    </Card>
  ),
};

export const Simple: Story = {
  render: () => (
    <Card className="w-[300px] p-6">
      <h3 className="font-semibold">Simple Card</h3>
      <p className="mt-2 text-sm text-muted-foreground">
        A minimal card without compound sub-components.
      </p>
    </Card>
  ),
};
