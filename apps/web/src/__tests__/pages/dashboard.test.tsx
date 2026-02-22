import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    className,
    "aria-label": ariaLabel,
    onClick,
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
    "aria-label"?: string;
    onClick?: React.MouseEventHandler;
  }) => (
    <a href={href} className={className} aria-label={ariaLabel} onClick={onClick}>
      {children}
    </a>
  ),
}));

vi.mock("@spacescale/ui", () => ({
  SearchInput: ({
    placeholder,
    value,
    onValueChange,
    className,
  }: {
    placeholder?: string;
    value?: string;
    onValueChange?: (v: string) => void;
    className?: string;
  }) => (
    <input
      type="search"
      placeholder={placeholder}
      value={value}
      onChange={(e) => onValueChange?.(e.target.value)}
      className={className}
      data-testid="search-input"
    />
  ),
}));

// ── Tests ─────────────────────────────────────────────────────────────────────

import DashboardPage from "@/app/(authenticated)/projects/page";

describe("DashboardPage", () => {
  describe("Initial render", () => {
    it("renders the search input", () => {
      render(<DashboardPage />);
      expect(screen.getByTestId("search-input")).toBeInTheDocument();
    });

    it("renders the grid view button", () => {
      render(<DashboardPage />);
      expect(screen.getByRole("button", { name: /grid view/i })).toBeInTheDocument();
    });

    it("renders the list view button", () => {
      render(<DashboardPage />);
      expect(screen.getByRole("button", { name: /list view/i })).toBeInTheDocument();
    });

    it("renders New Project link pointing to /projects/new", () => {
      render(<DashboardPage />);
      expect(screen.getByRole("link", { name: /new project/i })).toHaveAttribute("href", "/projects/new");
    });

    it("defaults to grid view (grid button is pressed)", () => {
      render(<DashboardPage />);
      expect(screen.getByRole("button", { name: /grid view/i })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: /list view/i })).toHaveAttribute("aria-pressed", "false");
    });
  });

  describe("Project listing", () => {
    it("renders all 6 mock projects", () => {
      render(<DashboardPage />);
      expect(screen.getAllByText(/silent-mountain|crimson-tide|neon-vector|oceanic-depth|lunar-orbit|void-runner/i)).toHaveLength(6);
    });

    it("renders silent-mountain project", () => {
      render(<DashboardPage />);
      expect(screen.getByText("silent-mountain")).toBeInTheDocument();
    });

    it("renders crimson-tide project", () => {
      render(<DashboardPage />);
      expect(screen.getByText("crimson-tide")).toBeInTheDocument();
    });

    it("renders neon-vector project", () => {
      render(<DashboardPage />);
      expect(screen.getByText("neon-vector")).toBeInTheDocument();
    });

    it("renders status labels for all statuses", () => {
      render(<DashboardPage />);
      expect(screen.getAllByText(/healthy/i).length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText(/warning/i).length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText(/critical/i).length).toBeGreaterThanOrEqual(1);
    });

    it("renders star buttons for each project", () => {
      render(<DashboardPage />);
      const starButtons = screen.getAllByRole("button", { name: /star project|unstar project/i });
      expect(starButtons.length).toBe(6);
    });

    it("silent-mountain starts as starred (Unstar project label)", () => {
      render(<DashboardPage />);
      const silentMountainStar = screen.getByLabelText("Unstar project");
      expect(silentMountainStar).toBeInTheDocument();
    });
  });

  describe("Grid view", () => {
    it("renders project cards as links in grid view", () => {
      render(<DashboardPage />);
      // In grid view, each project name is inside a <Link href=/projects/...>
      const link = screen.getByRole("link", { name: /silent-mountain/i });
      expect(link).toHaveAttribute("href", "/projects/silent-mountain");
    });

    it("renders resource counts in grid view", () => {
      render(<DashboardPage />);
      expect(screen.getByText("7 Resources")).toBeInTheDocument();
    });
  });

  describe("List view", () => {
    it("switches to list view on button click", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.click(screen.getByRole("button", { name: /list view/i }));

      expect(screen.getByRole("button", { name: /list view/i })).toHaveAttribute("aria-pressed", "true");
      expect(screen.getByRole("button", { name: /grid view/i })).toHaveAttribute("aria-pressed", "false");
    });

    it("still shows all project names in list view", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.click(screen.getByRole("button", { name: /list view/i }));

      expect(screen.getByText("silent-mountain")).toBeInTheDocument();
      expect(screen.getByText("void-runner")).toBeInTheDocument();
    });

    it("shows project links in list view", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.click(screen.getByRole("button", { name: /list view/i }));

      const links = screen.getAllByRole("link", { name: /silent-mountain/i });
      expect(links.length).toBeGreaterThanOrEqual(1);
      expect(links[0]).toHaveAttribute("href", "/projects/silent-mountain");
    });
  });

  describe("Search filtering", () => {
    it("filters projects by name", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.type(screen.getByTestId("search-input"), "silent");

      expect(screen.getByText("silent-mountain")).toBeInTheDocument();
      expect(screen.queryByText("crimson-tide")).not.toBeInTheDocument();
    });

    it("is case-insensitive", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.type(screen.getByTestId("search-input"), "NEON");

      expect(screen.getByText("neon-vector")).toBeInTheDocument();
    });

    it("shows empty state when no projects match", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.type(screen.getByTestId("search-input"), "xyznonexistent");

      expect(screen.getByText(/no projects match/i)).toBeInTheDocument();
    });

    it("shows Clear search button in empty state", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.type(screen.getByTestId("search-input"), "xyznonexistent");

      expect(screen.getByRole("button", { name: /clear search/i })).toBeInTheDocument();
    });

    it("clears search and shows all projects when Clear search is clicked", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      await user.type(screen.getByTestId("search-input"), "xyznonexistent");
      await user.click(screen.getByRole("button", { name: /clear search/i }));

      expect(screen.getByText("silent-mountain")).toBeInTheDocument();
      expect(screen.getByText("void-runner")).toBeInTheDocument();
    });
  });

  describe("Star toggle", () => {
    it("toggles star from unstarred to starred", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      // crimson-tide starts unstarred — its star button says "Star project"
      const starBtns = screen.getAllByRole("button", { name: /^star project$/i });
      await user.click(starBtns[0]);

      // Now there should be one more "Unstar project" button
      expect(screen.getAllByRole("button", { name: /unstar project/i })).toHaveLength(2);
    });

    it("toggles star from starred to unstarred", async () => {
      const user = userEvent.setup();
      render(<DashboardPage />);

      // silent-mountain starts as starred
      await user.click(screen.getByRole("button", { name: /unstar project/i }));

      expect(screen.queryByRole("button", { name: /unstar project/i })).not.toBeInTheDocument();
    });
  });

  describe("Settings button per project", () => {
    it("renders a settings button for each project in grid view", () => {
      render(<DashboardPage />);
      const settingsBtns = screen.getAllByRole("button", { name: /project settings/i });
      expect(settingsBtns.length).toBe(6);
    });
  });
});
