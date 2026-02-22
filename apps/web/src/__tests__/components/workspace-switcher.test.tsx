import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("WorkspaceSwitcher", () => {
  const defaultProps = { currentWorkspace: "my-workspace" };

  describe("Trigger button", () => {
    it("renders the trigger button with workspace label", () => {
      render(<WorkspaceSwitcher {...defaultProps} />);
      expect(screen.getByRole("button", { name: /my-workspace/i })).toBeInTheDocument();
    });

    it("displays user@workspace-name format", () => {
      render(<WorkspaceSwitcher {...defaultProps} />);
      expect(screen.getByText("user@my-workspace")).toBeInTheDocument();
    });

    it("marks trigger as collapsed initially", () => {
      render(<WorkspaceSwitcher {...defaultProps} />);
      expect(screen.getByRole("button", { expanded: false })).toBeInTheDocument();
    });
  });

  describe("Dropdown open/close", () => {
    it("opens dropdown on trigger click", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));

      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });

    it("closes dropdown on second click", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);
      const trigger = screen.getByRole("button", { name: /my-workspace/i });

      await user.click(trigger);
      await user.click(trigger);

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    it("closes dropdown on Escape key", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));
      expect(screen.getByRole("listbox")).toBeInTheDocument();

      await user.keyboard("{Escape}");
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    it("closes dropdown on click outside", async () => {
      render(
        <div>
          <WorkspaceSwitcher {...defaultProps} />
          <button type="button">Outside</button>
        </div>,
      );
      const user = userEvent.setup();

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));
      expect(screen.getByRole("listbox")).toBeInTheDocument();

      fireEvent.mouseDown(screen.getByRole("button", { name: /outside/i }));
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
  });

  describe("Workspace list", () => {
    it("renders all three workspaces when open", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher currentWorkspace="alpha-workspace" />);

      await user.click(screen.getByRole("button", { name: /alpha-workspace/i }));

      const options = screen.getAllByRole("option");
      expect(options).toHaveLength(3);
    });

    it("shows the current workspace name as first option", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher currentWorkspace="alpha-workspace" />);

      await user.click(screen.getByRole("button", { name: /alpha-workspace/i }));

      expect(screen.getByRole("option", { name: /alpha-workspace/i })).toBeInTheDocument();
    });

    it("marks the active workspace as selected", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));

      const options = screen.getAllByRole("option");
      const activeOption = options.find((opt) => opt.getAttribute("aria-selected") === "true");
      expect(activeOption).toBeDefined();
    });

    it("renders 'Create Workspace' button", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));

      expect(screen.getByRole("button", { name: /create workspace/i })).toBeInTheDocument();
    });

    it("closes dropdown when a workspace option is clicked", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));
      const options = screen.getAllByRole("option");
      await user.click(options[0]);

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    it("closes dropdown when 'Create Workspace' is clicked", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);

      await user.click(screen.getByRole("button", { name: /my-workspace/i }));
      await user.click(screen.getByRole("button", { name: /create workspace/i }));

      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });
  });

  describe("Accessibility", () => {
    it("has aria-haspopup attribute", () => {
      render(<WorkspaceSwitcher {...defaultProps} />);
      expect(screen.getByRole("button", { name: /my-workspace/i })).toHaveAttribute(
        "aria-haspopup",
        "listbox",
      );
    });

    it("sets aria-expanded correctly", async () => {
      const user = userEvent.setup();
      render(<WorkspaceSwitcher {...defaultProps} />);
      const trigger = screen.getByRole("button", { name: /my-workspace/i });

      expect(trigger).toHaveAttribute("aria-expanded", "false");
      await user.click(trigger);
      expect(trigger).toHaveAttribute("aria-expanded", "true");
    });
  });
});
