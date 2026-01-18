"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Rocket,
  Settings,
  LogOut,
  Menu,
  X,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui";
import { useAuth } from "@/lib/hooks";
import { useUIStore } from "@/stores/ui";

const navItems = [
  { href: "/app", label: "Dashboard", icon: LayoutDashboard },
  { href: "/app/new", label: "Deploy New", icon: Rocket },
];

export function Sidebar() {
  const pathname = usePathname();
  const { logout, user } = useAuth();
  const { sidebarOpen, toggleSidebar, mobileMenuOpen, setMobileMenuOpen } = useUIStore();

  return (
    <>
      {/* Mobile menu button */}
      <Button
        variant="ghost"
        size="icon"
        className="fixed left-4 top-4 z-50 md:hidden"
        onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
        aria-label={mobileMenuOpen ? "Close menu" : "Open menu"}
      >
        {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
      </Button>

      {/* Mobile overlay */}
      {mobileMenuOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 md:hidden"
          onClick={() => setMobileMenuOpen(false)}
          aria-hidden="true"
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          "fixed left-0 top-0 z-40 h-screen border-r bg-background transition-all duration-300",
          sidebarOpen ? "w-64" : "w-16",
          mobileMenuOpen ? "translate-x-0" : "-translate-x-full md:translate-x-0"
        )}
      >
        <div className="flex h-full flex-col">
          {/* Logo */}
          <div className="flex h-16 items-center justify-between border-b px-4">
            {sidebarOpen && (
              <Link href="/app" className="flex items-center space-x-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary">
                  <Rocket className="h-5 w-5 text-primary-foreground" />
                </div>
                <span className="text-xl font-bold">PaaS</span>
              </Link>
            )}
            <Button
              variant="ghost"
              size="icon"
              className="hidden md:flex"
              onClick={toggleSidebar}
              aria-label={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"}
            >
              {sidebarOpen ? (
                <ChevronLeft className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </Button>
          </div>

          {/* Navigation */}
          <nav className="flex-1 space-y-1 p-2">
            {navItems.map((item) => {
              const isActive = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={() => setMobileMenuOpen(false)}
                  className={cn(
                    "flex items-center rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  )}
                >
                  <item.icon className="h-5 w-5" />
                  {sidebarOpen && <span className="ml-3">{item.label}</span>}
                </Link>
              );
            })}
          </nav>

          {/* User section */}
          <div className="border-t p-2">
            {user && sidebarOpen && (
              <div className="mb-2 flex items-center space-x-3 px-3 py-2">
                {user.image ? (
                  <img
                    src={user.image}
                    alt={user.name || "User"}
                    className="h-8 w-8 rounded-full"
                  />
                ) : (
                  <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted">
                    {user.name?.[0] || user.email?.[0] || "U"}
                  </div>
                )}
                <div className="flex-1 truncate">
                  <p className="truncate text-sm font-medium">{user.name}</p>
                  <p className="truncate text-xs text-muted-foreground">{user.email}</p>
                </div>
              </div>
            )}
            <Button
              variant="ghost"
              className={cn("w-full", sidebarOpen ? "justify-start" : "justify-center")}
              onClick={logout}
            >
              <LogOut className="h-5 w-5" />
              {sidebarOpen && <span className="ml-3">Sign out</span>}
            </Button>
          </div>
        </div>
      </aside>
    </>
  );
}
