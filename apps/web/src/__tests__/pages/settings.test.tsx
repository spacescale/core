import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import SettingsPage from "@/app/(authenticated)/projects/[projectId]/settings/page";

describe("SettingsPage", () => {
  it("renders the Settings heading", () => {
    render(<SettingsPage />);
    expect(screen.getByRole("heading", { name: /settings/i })).toBeInTheDocument();
  });

  it("renders the coming-soon description", () => {
    render(<SettingsPage />);
    expect(screen.getByText(/coming soon/i)).toBeInTheDocument();
  });

  it("mentions project settings in the description", () => {
    render(<SettingsPage />);
    expect(screen.getByText(/project settings/i)).toBeInTheDocument();
  });
});
