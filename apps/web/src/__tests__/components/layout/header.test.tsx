import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

// ── Mocks ─────────────────────────────────────────────────────────────────────

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    className,
    "aria-label": ariaLabel,
  }: {
    href: string;
    children: React.ReactNode;
    className?: string;
    "aria-label"?: string;
  }) => (
    <a href={href} className={className} aria-label={ariaLabel}>
      {children}
    </a>
  ),
}));

vi.mock("next-themes", () => ({
  useTheme: vi.fn(() => ({ theme: "light", setTheme: vi.fn() })),
}));

vi.mock("@spacescale/ui", () => ({
  LogoMark: () => <div data-testid="logo-mark" />,
}));

const mockUseAuth = vi.fn();
vi.mock("@/lib/hooks", () => ({
  useAuth: () => mockUseAuth(),
}));

// ── Tests ─────────────────────────────────────────────────────────────────────

import { Header } from "@/components/layout/header";

const authWithUser = (overrides = {}) => ({
  isLoading: false,
  isAuthenticated: true,
  isUnauthenticated: false,
  user: { name: "Jane Smith", email: "jane@example.com", image: null },
  loginWithGithub: vi.fn(),
  loginWithGoogle: vi.fn(),
  logout: vi.fn(),
  ...overrides,
});

describe("Header", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue(authWithUser());
  });

  describe("Branding", () => {
    it("renders SpaceScale text", () => {
      render(<Header />);
      expect(screen.getByText(/spacescale/i)).toBeInTheDocument();
    });

    it("renders the logo mark", () => {
      render(<Header />);
      expect(screen.getByTestId("logo-mark")).toBeInTheDocument();
    });

    it("home link points to /projects", () => {
      render(<Header />);
      expect(screen.getByRole("link", { name: /spacescale home/i })).toHaveAttribute(
        "href",
        "/projects",
      );
    });
  });

  describe("User avatar", () => {
    it("shows initials when no user image", () => {
      mockUseAuth.mockReturnValue(authWithUser({ user: { name: "Jane Smith", email: "jane@example.com", image: null } }));
      render(<Header />);
      // Initials from "Jane Smith" → "JS"
      expect(screen.getByRole("button", { name: /user menu/i })).toHaveTextContent("JS");
    });

    it("shows 'U' when user has no name or email", () => {
      mockUseAuth.mockReturnValue(authWithUser({ user: { name: null, email: null, image: null } }));
      render(<Header />);
      expect(screen.getByRole("button", { name: /user menu/i })).toHaveTextContent("U");
    });

    it("derives initials from email when name is absent", () => {
      mockUseAuth.mockReturnValue(
        authWithUser({ user: { name: null, email: "zack@example.com", image: null } }),
      );
      render(<Header />);
      expect(screen.getByRole("button", { name: /user menu/i })).toHaveTextContent("Z");
    });

    it("renders user image when available", () => {
      mockUseAuth.mockReturnValue(
        authWithUser({ user: { name: "Jane Smith", email: "jane@example.com", image: "https://example.com/avatar.jpg" } }),
      );
      render(<Header />);
      const img = screen.getByRole("img", { name: /jane smith/i });
      expect(img).toHaveAttribute("src", "https://example.com/avatar.jpg");
    });
  });

  describe("Actions", () => {
    it("renders notification bell", () => {
      render(<Header />);
      expect(screen.getByRole("button", { name: /notifications/i })).toBeInTheDocument();
    });

    it("renders theme toggle button", () => {
      render(<Header />);
      expect(screen.getByRole("button", { name: /switch to dark mode/i })).toBeInTheDocument();
    });

    it("renders PRO PLAN badge", () => {
      render(<Header />);
      expect(screen.getByText(/pro plan/i)).toBeInTheDocument();
    });
  });
});
