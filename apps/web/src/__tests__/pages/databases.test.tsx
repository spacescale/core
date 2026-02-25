import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import DatabasesPage from "@/app/(authenticated)/projects/[projectId]/databases/page";

describe("DatabasesPage", () => {
  it("renders the Databases heading", () => {
    render(<DatabasesPage />);
    expect(screen.getByRole("heading", { name: /databases/i })).toBeInTheDocument();
  });

  it("renders the coming-soon description", () => {
    render(<DatabasesPage />);
    expect(screen.getByText(/coming soon/i)).toBeInTheDocument();
  });

  it("mentions Postgres or managed databases in the description", () => {
    render(<DatabasesPage />);
    expect(screen.getByText(/postgres/i)).toBeInTheDocument();
  });
});
