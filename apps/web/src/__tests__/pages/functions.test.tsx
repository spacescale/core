import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import FunctionsPage from "@/app/(authenticated)/projects/[projectId]/functions/page";

describe("FunctionsPage", () => {
  it("renders the Functions heading", () => {
    render(<FunctionsPage />);
    expect(screen.getByRole("heading", { name: /functions/i })).toBeInTheDocument();
  });

  it("renders the coming-soon description", () => {
    render(<FunctionsPage />);
    expect(screen.getByText(/coming soon/i)).toBeInTheDocument();
  });

  it("mentions edge functions or serverless in the description", () => {
    render(<FunctionsPage />);
    expect(screen.getByText(/serverless functions/i)).toBeInTheDocument();
  });
});
