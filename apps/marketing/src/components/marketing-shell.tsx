import type { ReactNode } from "react";
import Link from "next/link";
import { LogoMark } from "@spacescale/ui";

const DEFAULT_DASHBOARD_URL = "http://localhost:3000";

function sanitizeBaseUrl(url: string): string {
  return url.replace(/\/+$/, "");
}

type MarketingShellProps = {
  title: string;
  description: string;
  children: ReactNode;
};

export function MarketingShell({
  title,
  description,
  children,
}: MarketingShellProps) {
  const dashboardBaseUrl = sanitizeBaseUrl(
    process.env.NEXT_PUBLIC_DASHBOARD_URL ?? DEFAULT_DASHBOARD_URL,
  );
  const loginHref = `${dashboardBaseUrl}/login`;
  const deployHref = loginHref;
  const currentYear = new Date().getFullYear();

  return (
    <div className="relative flex min-h-screen flex-col bg-railway-bg text-railway-text font-display antialiased overflow-x-hidden">
      <div className="fixed inset-0 bg-square-grid bg-grid-pattern pointer-events-none z-0 opacity-[0.8] grid-mask" />

      <header className="fixed top-0 z-50 w-full border-b hairline-border border-railway-border/40 bg-railway-bg/70 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
          <Link href="/" className="flex items-center gap-3">
            <LogoMark />
            <span className="hidden text-lg font-bold tracking-tight text-white md:block">
              SpaceScale
            </span>
          </Link>
          <nav className="hidden items-center gap-8 md:flex">
            <Link
              className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
              href="/"
            >
              Platform
            </Link>
            <Link
              className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
              href="/"
            >
              Solutions
            </Link>
            <Link
              className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
              href="/pricing"
            >
              Pricing
            </Link>
            <Link
              className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
              href="/terms"
            >
              Docs
            </Link>
          </nav>
          <div className="flex items-center gap-4">
            <a
              className="text-sm font-medium text-railway-muted transition-colors hover:text-white"
              href={loginHref}
            >
              Login
            </a>
            <a
              className="group relative overflow-hidden rounded-full border hairline-border border-white/20 bg-white/[0.03] px-5 py-2 transition-all duration-300 hover:border-white/40 hover:bg-white/[0.08]"
              href={deployHref}
            >
              <span className="relative z-10 text-sm font-medium tracking-wide text-off-white transition-colors group-hover:text-white">
                Deploy Now
              </span>
            </a>
          </div>
        </div>
      </header>

      <main className="relative z-10 mx-auto w-full max-w-5xl flex-1 px-6 pb-24 pt-32">
        <div className="mb-10 max-w-3xl space-y-4">
          <h1 className="text-4xl md:text-5xl font-[200] tracking-tight text-off-white leading-[1.1]">
            {title}
          </h1>
          <p className="text-lg text-railway-muted font-light leading-relaxed">
            {description}
          </p>
        </div>
        <div className="space-y-6">{children}</div>
      </main>

      <footer className="border-t hairline-border border-white/5 bg-[#0b0d14] py-10">
        <div className="mx-auto flex max-w-7xl flex-col items-start justify-between gap-8 px-6 md:flex-row">
          <div>
            <div className="mb-4 flex items-center gap-2">
              <LogoMark className="scale-90 opacity-80" />
              <span className="text-sm font-light text-off-white">
                SpaceScale
              </span>
            </div>
            <p className="max-w-xs text-xs font-light text-railway-muted">
              Infrastructure for modern engineering teams. <br />
              Ontario, Canada.
            </p>
          </div>
          <div className="flex gap-12">
            <div className="flex flex-col gap-3">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
                Product
              </span>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/"
              >
                Features
              </Link>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/pricing"
              >
                Pricing
              </Link>
            </div>
            <div className="flex flex-col gap-3">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
                Company
              </span>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/"
              >
                About
              </Link>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/careers"
              >
                Careers
              </Link>
            </div>
            <div className="flex flex-col gap-3">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
                Legal
              </span>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/privacy"
              >
                Privacy
              </Link>
              <Link
                className="text-xs text-railway-muted transition-colors hover:text-off-white"
                href="/terms"
              >
                Terms
              </Link>
            </div>
          </div>
        </div>
        <div className="mx-auto mt-8 flex max-w-7xl items-center justify-between border-t border-white/5 px-6 pt-6">
          <p className="text-[10px] uppercase text-railway-muted">
            &copy; {currentYear} SpaceScale Inc.
          </p>
          <div className="flex items-center gap-2">
            <span className="size-1.5 rounded-full bg-indigo-500 shadow-[0_0_5px_rgba(99,102,241,0.8)]" />
            <span className="text-[10px] font-mono uppercase text-railway-muted">
              Systems Normal
            </span>
          </div>
        </div>
      </footer>
    </div>
  );
}
