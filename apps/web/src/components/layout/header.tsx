"use client";

import Link from "next/link";
import { Bell, Menu } from "lucide-react";
import { LogoMark } from "@spacescale/ui";
import { ThemeToggle } from "@/components/theme-toggle";
import { useAuth } from "@/lib/hooks";
import { cn } from "@/lib/utils";

interface HeaderProps {
  onMenuClick?: () => void;
}

export function Header({ onMenuClick }: HeaderProps) {
  const { user } = useAuth();

  const initials = user?.name
    ? user.name
        .split(" ")
        .map((n: string) => n[0])
        .join("")
        .toUpperCase()
        .slice(0, 2)
    : user?.email?.[0]?.toUpperCase() ?? "U";

  return (
    <header
      className={cn(
        "fixed top-0 left-0 right-0 z-50 h-14",
        "flex items-center justify-between px-4 md:px-6",
        "bg-[rgba(248,249,250,0.85)] dark:bg-[rgba(15,17,26,0.7)]",
        "backdrop-blur-[12px]",
        "border-b border-black/[0.06] dark:border-white/[0.05]"
      )}
    >
      {/* Left: hamburger (mobile) + Logo */}
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={onMenuClick}
          aria-label="Open menu"
          className="md:hidden flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/10 transition-colors"
        >
          <Menu className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <Link
          href="/projects"
          className="flex items-end gap-3 pb-[3px] select-none"
          aria-label="SpaceScale home"
        >
          <LogoMark className="h-6" />
          <span className="text-sm font-[200] tracking-[0.2em] uppercase leading-none text-foreground">
            SpaceScale
          </span>
        </Link>
      </div>

      {/* Right: Actions */}
      <div className="flex items-center gap-3">
        <div className="hidden sm:flex items-center px-2 py-0.5 rounded border border-primary/20 bg-primary/10">
          <span className="text-[10px] font-bold text-primary tracking-wider">
            PRO PLAN
          </span>
        </div>

        <button
          className="relative flex h-8 w-8 items-center justify-center rounded-full text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/10 transition-colors"
          aria-label="Notifications"
        >
          <Bell className="h-[18px] w-[18px]" strokeWidth={1.5} />
          <span className="absolute top-1.5 right-1.5 h-1.5 w-1.5 rounded-full bg-primary border border-background" />
        </button>

        <ThemeToggle />

        <button
          className={cn(
            "h-8 w-8 rounded-full flex items-center justify-center",
            "text-white text-xs font-medium",
            "bg-gradient-to-tr from-primary to-purple-500",
            "border border-white/20 dark:border-white/10 shadow-sm",
            "cursor-pointer hover:ring-2 ring-primary/30 transition-all"
          )}
          aria-label="User menu"
        >
          {user?.image ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={user.image}
              alt={user.name || "User"}
              className="h-full w-full rounded-full object-cover"
            />
          ) : (
            initials
          )}
        </button>
      </div>
    </header>
  );
}
