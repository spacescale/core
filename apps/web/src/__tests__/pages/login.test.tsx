import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// ── Mocks ─────────────────────────────────────────────────────────────────────

const mockRouterReplace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: vi.fn(() => ({ replace: mockRouterReplace, push: vi.fn() })),
}));

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

vi.mock("next-themes", () => ({
  useTheme: vi.fn(() => ({ theme: "light", setTheme: vi.fn() })),
}));

vi.mock("@spacescale/ui", () => ({
  LogoMark: () => <div data-testid="logo-mark" />,
  Badge: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <span className={className}>{children}</span>
  ),
  Button: ({
    children,
    onClick,
    disabled,
    className,
    type,
    "aria-label": ariaLabel,
  }: {
    children: React.ReactNode;
    onClick?: () => void;
    disabled?: boolean;
    className?: string;
    type?: "button" | "submit" | "reset";
    "aria-label"?: string;
  }) => (
    <button
      type={type ?? "button"}
      onClick={onClick}
      disabled={disabled}
      className={className}
      aria-label={ariaLabel}
    >
      {children}
    </button>
  ),
  Card: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
  CardContent: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));

const mockLoginWithGithub = vi.fn();
const mockLoginWithGoogle = vi.fn();
const mockUseAuth = vi.fn();

vi.mock("@/lib/hooks", () => ({
  useAuth: () => mockUseAuth(),
}));

// ── Tests ─────────────────────────────────────────────────────────────────────

import LoginPage from "@/app/login/page";

const unauthenticatedAuth = (overrides = {}) => ({
  isLoading: false,
  isAuthenticated: false,
  isUnauthenticated: true,
  user: null,
  loginWithGithub: mockLoginWithGithub,
  loginWithGoogle: mockLoginWithGoogle,
  logout: vi.fn(),
  ...overrides,
});

describe("LoginPage", () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue(unauthenticatedAuth());
    mockRouterReplace.mockClear();
    mockLoginWithGithub.mockClear();
    mockLoginWithGoogle.mockClear();
  });

  describe("Loading state", () => {
    it("shows a loading spinner while auth is loading", () => {
      mockUseAuth.mockReturnValue(unauthenticatedAuth({ isLoading: true, isUnauthenticated: false }));
      const { container } = render(<LoginPage />);
      // Spinner is an animated div — check for the animate-spin class
      expect(container.querySelector(".animate-spin")).toBeInTheDocument();
    });

    it("shows a spinner when already authenticated (before redirect)", () => {
      mockUseAuth.mockReturnValue(
        unauthenticatedAuth({ isAuthenticated: true, isUnauthenticated: false }),
      );
      const { container } = render(<LoginPage />);
      expect(container.querySelector(".animate-spin")).toBeInTheDocument();
    });
  });

  describe("Redirect behaviour", () => {
    it("redirects to /projects when already authenticated", () => {
      mockUseAuth.mockReturnValue(
        unauthenticatedAuth({ isAuthenticated: true, isUnauthenticated: false }),
      );
      render(<LoginPage />);
      expect(mockRouterReplace).toHaveBeenCalledWith("/projects");
    });

    it("does not redirect when unauthenticated", () => {
      render(<LoginPage />);
      expect(mockRouterReplace).not.toHaveBeenCalled();
    });
  });

  describe("Rendered form", () => {
    it("renders the Sign in heading", () => {
      render(<LoginPage />);
      expect(screen.getByRole("heading", { name: /sign in/i })).toBeInTheDocument();
    });

    it("renders the SpaceScale logo mark", () => {
      render(<LoginPage />);
      expect(screen.getByTestId("logo-mark")).toBeInTheDocument();
    });

    it("renders the SpaceScale brand name", () => {
      render(<LoginPage />);
      expect(screen.getByText(/spacescale/i)).toBeInTheDocument();
    });

    it("renders the GitHub sign-in button", () => {
      render(<LoginPage />);
      expect(
        screen.getByRole("button", { name: /continue with github/i }),
      ).toBeInTheDocument();
    });

    it("renders the Google sign-in button", () => {
      render(<LoginPage />);
      expect(
        screen.getByRole("button", { name: /continue with google/i }),
      ).toBeInTheDocument();
    });

    it("renders the theme toggle", () => {
      render(<LoginPage />);
      expect(screen.getByRole("button", { name: /switch to dark mode/i })).toBeInTheDocument();
    });

    it("renders Docs link", () => {
      render(<LoginPage />);
      expect(screen.getByRole("link", { name: /docs/i })).toBeInTheDocument();
    });

    it("renders Articles link", () => {
      render(<LoginPage />);
      expect(screen.getByRole("link", { name: /articles/i })).toBeInTheDocument();
    });

    it("shows 'Trusted by developers at' section", () => {
      render(<LoginPage />);
      expect(screen.getByText(/trusted by developers at/i)).toBeInTheDocument();
    });
  });

  describe("Auth actions", () => {
    it("calls loginWithGithub when GitHub button is clicked", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);
      await user.click(screen.getByRole("button", { name: /continue with github/i }));
      expect(mockLoginWithGithub).toHaveBeenCalledOnce();
    });

    it("calls loginWithGoogle when Google button is clicked", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);
      await user.click(screen.getByRole("button", { name: /continue with google/i }));
      expect(mockLoginWithGoogle).toHaveBeenCalledOnce();
    });
  });
});
