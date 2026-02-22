"use client";

import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import Link from "next/link";
import { Header } from "./header";
import { Sidebar } from "./sidebar";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";
import { useAuth } from "@/lib/hooks";
import { Skeleton } from "@/components/ui";
import { cn } from "@/lib/utils";

interface AppShellProps {
  children: React.ReactNode;
}

interface BreadcrumbSegment {
  label: string;
  href?: string;
}

/**
 * Build a logical breadcrumb from the Next.js pathname.
 *
 * Hierarchy:
 *   /projects                              → Projects
 *   /projects/new                          → Projects / new
 *   /projects/[id]                         → Projects / [id] / Applications
 *   /projects/[id]/workers                 → Projects / [id] / Workers
 *   /projects/[id]/functions               → Projects / [id] / Functions
 *   /projects/[id]/databases               → Projects / [id] / Databases
 *   /projects/[id]/settings                → Projects / [id] / Settings
 *   /projects/[id]/applications/[appId]    → Projects / [id] / Applications / [appId]
 */
function useBreadcrumb(pathname: string): BreadcrumbSegment[] {
  const path = pathname.replace(/^\/projects\/?/, "");
  const segments = path.split("/").filter(Boolean);

  // /projects
  if (segments.length === 0) {
    return [{ label: "projects", href: "/projects" }];
  }

  const first = segments[0];

  // /projects/new
  if (first === "new") {
    return [
      { label: "projects", href: "/projects" },
      { label: "new" },
    ];
  }

  // /projects/[projectId]/...
  const projectId = first;
  const projectHref = `/projects/${projectId}`;

  if (segments.length === 1) {
    return [
      { label: "projects", href: "/projects" },
      { label: projectId, href: projectHref },
      { label: "applications" },
    ];
  }

  const section = segments[1];

  // /projects/[projectId]/{workers,functions,databases,settings}
  if (["workers", "functions", "databases", "settings"].includes(section)) {
    return [
      { label: "projects",  href: "/projects" },
      { label: projectId,   href: projectHref },
      { label: section },
    ];
  }

  // /projects/[projectId]/applications/[appId]/...
  if (section === "applications") {
    const crumbs: BreadcrumbSegment[] = [
      { label: "projects",     href: "/projects" },
      { label: projectId,      href: projectHref },
      { label: "applications", href: projectHref },
    ];
    let hrefBuilder = `${projectHref}/applications`;
    for (const seg of segments.slice(2)) {
      hrefBuilder = `${hrefBuilder}/${seg}`;
      crumbs.push({ label: seg, href: hrefBuilder });
    }
    return crumbs;
  }

  // Fallback
  return [
    { label: "projects", href: "/projects" },
    { label: projectId,  href: projectHref },
    { label: section },
  ];
}

export function AppShell({ children }: AppShellProps) {
  const { isLoading, isUnauthenticated, user } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const breadcrumb = useBreadcrumb(pathname);
  const [mobileOpen, setMobileOpen] = useState(false);

  const workspaceName =
    user?.name?.toLowerCase().replace(/\s+/g, "-") ??
    user?.email?.split("@")[0]?.replace(/[^a-z0-9-]/g, "-") ??
    "my-workspace";

  useEffect(() => {
    if (isUnauthenticated) {
      router.push("/login");
    }
  }, [isUnauthenticated, router]);

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-3">
          <Skeleton className="h-10 w-10 rounded-lg" />
          <Skeleton className="h-3 w-28 mt-2" />
        </div>
      </div>
    );
  }

  if (isUnauthenticated) {
    return null;
  }

  return (
    <div className="relative min-h-screen bg-background">
      {/* Subtle grid pattern — light mode */}
      <div
        className="pointer-events-none fixed inset-0 z-0 dark:hidden"
        style={{
          backgroundImage:
            "linear-gradient(to right, rgba(0,0,0,0.03) 1px, transparent 1px), linear-gradient(to bottom, rgba(0,0,0,0.03) 1px, transparent 1px)",
          backgroundSize: "40px 40px",
        }}
        aria-hidden="true"
      />
      {/* Grid pattern — dark mode */}
      <div
        className="pointer-events-none fixed inset-0 z-0 hidden dark:block"
        style={{
          backgroundImage:
            "linear-gradient(to right, rgba(255,255,255,0.04) 1px, transparent 1px), linear-gradient(to bottom, rgba(255,255,255,0.04) 1px, transparent 1px)",
          backgroundSize: "40px 40px",
        }}
        aria-hidden="true"
      />

      {/* Fixed header h-14 */}
      <Header onMenuClick={() => setMobileOpen(true)} />

      {/* Fixed subheader h-9 — terminal breadcrumb + workspace switcher */}
      <div
        className={cn(
          "fixed top-14 left-0 right-0 z-40 h-9",
          "flex items-center px-6",
          "font-mono text-[12px]",
          "bg-[rgba(248,249,250,0.65)] dark:bg-background",
          "backdrop-blur-[8px] dark:backdrop-blur-none",
          "border-b border-black/[0.06] dark:border-white/[0.05]"
        )}
      >
        {/* Scrollable row — all items stay on one line; hidden scrollbar lets mobile users swipe to see full path */}
        <div className="flex items-center gap-2 w-full overflow-x-auto no-scrollbar">
          {/* Workspace switcher — flex-shrink-0 so it never gets squished or wraps */}
          <div className="flex-shrink-0">
            <WorkspaceSwitcher currentWorkspace={workspaceName} />
          </div>

          <span className="text-muted-foreground/50 font-light select-none flex-shrink-0">%</span>

          {/* Breadcrumb path segments — flex-shrink-0 so they never get clipped */}
          <div className="flex items-center gap-1.5 whitespace-nowrap select-none flex-shrink-0">
            {breadcrumb.map((segment, i) => {
              const isLast = i === breadcrumb.length - 1;
              return (
                <span key={`${segment.label}-${i}`} className="flex items-center gap-1.5 flex-shrink-0">
                  {i > 0 && (
                    <span className="text-muted-foreground/30">/</span>
                  )}
                  {isLast || !segment.href ? (
                    <span
                      className={cn(
                        "tracking-wide px-1 py-0.5 rounded",
                        isLast
                          ? "text-primary bg-primary/10 border border-primary/20 font-[400]"
                          : "text-muted-foreground font-[300]"
                      )}
                    >
                      {segment.label}
                    </span>
                  ) : (
                    <Link
                      href={segment.href}
                      className="font-[300] tracking-wide transition-colors px-1 py-0.5 rounded text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/5"
                    >
                      {segment.label}
                    </Link>
                  )}
                </span>
              );
            })}
          </div>
        </div>
      </div>

      {/* Body layout: sidebar + main, offset by header+subheader (92px) */}
      <div className="relative z-10 flex pt-[92px] min-h-[calc(100vh-92px)]">
        <Sidebar mobileOpen={mobileOpen} onClose={() => setMobileOpen(false)} />
        <main className="flex-1 min-h-full md:ml-64 transition-all duration-300">
          <div className="p-6 lg:p-8">{children}</div>
        </main>
      </div>
    </div>
  );
}
