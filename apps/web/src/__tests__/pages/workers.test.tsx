import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import WorkersPage from "@/app/(authenticated)/projects/[projectId]/workers/page";

describe("WorkersPage", () => {
  it("renders the Workers heading", () => {
    render(<WorkersPage />);
    expect(screen.getByRole("heading", { name: /workers/i })).toBeInTheDocument();
  });

  it("renders the coming-soon description", () => {
    render(<WorkersPage />);
    expect(screen.getByText(/coming soon/i)).toBeInTheDocument();
  });

  it("mentions long-running tasks in the description", () => {
    render(<WorkersPage />);
    expect(screen.getByText(/long-running tasks/i)).toBeInTheDocument();
  });
});
