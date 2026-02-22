import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    className,
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
  }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

const mockPathname = vi.fn(() => "/projects");
vi.mock("next/navigation", () => ({
  usePathname: () => mockPathname(),
}));

const mockLogout = vi.fn();
const mockUseAuth = vi.fn();
vi.mock("@/lib/hooks", () => ({
  useAuth: () => mockUseAuth(),
}));

// ── Tests ─────────────────────────────────────────────────────────────────────

import { Sidebar } from "@/components/layout/sidebar";

const defaultAuth = (overrides = {}) => ({
  isLoading: false,
  isAuthenticated: true,
  isUnauthenticated: false,
  user: { name: "Sam Developer", email: "sam@example.com" },
  logout: mockLogout,
  ...overrides,
});

describe("Sidebar", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue(defaultAuth());
    mockLogout.mockClear();
    mockPathname.mockReturnValue("/projects");
  });

  describe("Workspace display", () => {
    it("renders the workspace name derived from user name", () => {
      render(<Sidebar />);
      // "Sam Developer" → "sam-developer"
      expect(screen.getByText("sam-developer")).toBeInTheDocument();
    });

    it("derives workspace name from email when no name", () => {
      mockUseAuth.mockReturnValue(
        defaultAuth({ user: { name: null, email: "alice@example.com" } }),
      );
      render(<Sidebar />);
      expect(screen.getByText("alice")).toBeInTheDocument();
    });

    it("falls back to 'my-workspace' when no user info", () => {
      mockUseAuth.mockReturnValue(defaultAuth({ user: null }));
      render(<Sidebar />);
      expect(screen.getByText("my-workspace")).toBeInTheDocument();
    });

    it("renders a workspace ID in mono font", () => {
      render(<Sidebar />);
      expect(screen.getByText(/^ID:/)).toBeInTheDocument();
    });
  });

  describe("Navigation items", () => {
    it("renders Applications nav item", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /applications/i })).toBeInTheDocument();
    });

    it("renders Workers nav item", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /workers/i })).toBeInTheDocument();
    });

    it("renders Functions nav item", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /functions/i })).toBeInTheDocument();
    });

    it("renders Databases nav item", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /databases/i })).toBeInTheDocument();
    });

    // On the projects listing page (/projects), Applications points to /projects
    it("Applications link points to /projects on the listing page", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /applications/i })).toHaveAttribute(
        "href",
        "/projects",
      );
    });

    // Workers/Functions/Databases are disabled (#) when no project is active
    it("Workers link is disabled (href=#) when no project is active", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /workers/i })).toHaveAttribute("href", "#");
    });

    it("Functions link is disabled (href=#) when no project is active", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /functions/i })).toHaveAttribute("href", "#");
    });

    it("Databases link is disabled (href=#) when no project is active", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /databases/i })).toHaveAttribute("href", "#");
    });

    // Inside a project, resource links become project-scoped
    it("Applications link points to /projects/abc123 inside a project", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /applications/i })).toHaveAttribute(
        "href",
        "/projects/abc123",
      );
    });

    it("Workers link points to /projects/abc123/workers inside a project", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /workers/i })).toHaveAttribute(
        "href",
        "/projects/abc123/workers",
      );
    });

    it("Functions link points to /projects/abc123/functions inside a project", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /functions/i })).toHaveAttribute(
        "href",
        "/projects/abc123/functions",
      );
    });

    it("Databases link points to /projects/abc123/databases inside a project", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /databases/i })).toHaveAttribute(
        "href",
        "/projects/abc123/databases",
      );
    });
  });

  describe("Active state", () => {
    it("marks Applications active on /projects", () => {
      mockPathname.mockReturnValue("/projects");
      render(<Sidebar />);
      const appLink = screen.getByRole("link", { name: /applications/i });
      // Active item gets border class
      expect(appLink.className).toMatch(/border/);
    });

    it("marks Applications active on /projects/abc123", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      const appLink = screen.getByRole("link", { name: /applications/i });
      expect(appLink.className).toMatch(/border/);
    });

    it("marks Workers active on /projects/abc123/workers", () => {
      mockPathname.mockReturnValue("/projects/abc123/workers");
      render(<Sidebar />);
      const workersLink = screen.getByRole("link", { name: /workers/i });
      expect(workersLink.className).toMatch(/border/);
    });
  });

  describe("Bottom section", () => {
    // Settings is disabled (#) on the listing page (no projectId)
    it("Settings link is disabled (href=#) on the projects listing page", () => {
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "#");
    });

    // Settings is enabled inside a project
    it("Settings link points to /projects/abc123/settings inside a project", () => {
      mockPathname.mockReturnValue("/projects/abc123");
      render(<Sidebar />);
      expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute(
        "href",
        "/projects/abc123/settings",
      );
    });

    it("renders Sign out button", () => {
      render(<Sidebar />);
      expect(screen.getByRole("button", { name: /sign out/i })).toBeInTheDocument();
    });

    it("calls logout when Sign out is clicked", async () => {
      const user = userEvent.setup();
      render(<Sidebar />);
      await user.click(screen.getByRole("button", { name: /sign out/i }));
      expect(mockLogout).toHaveBeenCalledOnce();
    });

    it("renders Collapse button", () => {
      render(<Sidebar />);
      expect(screen.getByRole("button", { name: /collapse sidebar/i })).toBeInTheDocument();
    });
  });

  describe("Collapse behaviour", () => {
    it("hides section labels when collapsed", async () => {
      const user = userEvent.setup();
      render(<Sidebar />);

      // Before collapse: workspace name visible
      expect(screen.getByText("sam-developer")).toBeInTheDocument();

      await user.click(screen.getByRole("button", { name: /collapse sidebar/i }));

      // After collapse: workspace section hidden
      expect(screen.queryByText("sam-developer")).not.toBeInTheDocument();
    });

    it("shows Expand button after collapsing", async () => {
      const user = userEvent.setup();
      render(<Sidebar />);

      await user.click(screen.getByRole("button", { name: /collapse sidebar/i }));

      expect(screen.getByRole("button", { name: /expand sidebar/i })).toBeInTheDocument();
    });

    it("re-expands when Expand is clicked", async () => {
      const user = userEvent.setup();
      render(<Sidebar />);

      await user.click(screen.getByRole("button", { name: /collapse sidebar/i }));
      await user.click(screen.getByRole("button", { name: /expand sidebar/i }));

      expect(screen.getByText("sam-developer")).toBeInTheDocument();
    });
  });
});
