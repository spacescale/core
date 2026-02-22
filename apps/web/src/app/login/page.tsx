"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Github } from "lucide-react";
import { Badge, Button, Card, CardContent, LogoMark } from "@spacescale/ui";
import { useAuth } from "@/lib/hooks";
import { ThemeToggle } from "@/components/theme-toggle";
import { cn } from "@/lib/utils";

const DEFAULT_MARKETING_URL = "http://localhost:3001";

function sanitizeBaseUrl(url: string): string {
  return url.replace(/\/+$/, "");
}

type TrustedRowProps = {
  ariaHidden?: boolean;
};

function TrustedRow({ ariaHidden = false }: TrustedRowProps) {
  return (
    <div
      aria-hidden={ariaHidden}
      className="flex min-w-full shrink-0 items-center justify-around gap-12 animate-auth-infinite-scroll"
    >
      <div className="flex items-center gap-2">
        <div className="flex h-4 w-4 items-center justify-center rounded-full border border-foreground/30">
          <div className="h-1.5 w-1.5 rounded-full bg-foreground/50" />
        </div>
        <span className="text-sm font-bold tracking-tight text-foreground/70">twilio</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-sm font-bold italic text-foreground/70">Alibaba.com</span>
      </div>
      <div className="flex items-center gap-2">
        <div className="h-3 w-3 rounded-full border border-dashed border-foreground/30" />
        <span className="text-xs font-semibold text-foreground/70">Bridge</span>
      </div>
      <div className="flex items-center gap-2">
        <div className="relative h-4 w-4 overflow-hidden rounded-full bg-foreground/10">
          <div className="absolute bottom-0 h-1/2 w-full bg-foreground/40" />
        </div>
        <span className="text-xs font-bold text-foreground/70">Base 44</span>
      </div>
      <div className="origin-left scale-75 leading-none">
        <span className="block text-[8px] font-bold tracking-widest text-foreground/70">CONTENT</span>
        <span className="block text-[8px] font-bold tracking-widest text-foreground/70">SQUARE</span>
      </div>
    </div>
  );
}

export default function LoginPage() {
  const { isAuthenticated, isLoading, loginWithGithub, loginWithGoogle } = useAuth();
  const router = useRouter();

  const marketingBaseUrl = sanitizeBaseUrl(
    process.env.NEXT_PUBLIC_MARKETING_URL ?? DEFAULT_MARKETING_URL,
  );
  const docsHref = `${marketingBaseUrl}/docs`;
  const articlesHref = `${marketingBaseUrl}/articles`;

  useEffect(() => {
    if (isAuthenticated) {
      router.replace("/projects");
    }
  }, [isAuthenticated, router]);

  if (isLoading || isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
      </div>
    );
  }

  return (
    <div className="relative flex min-h-dvh flex-col overflow-x-hidden bg-background text-foreground">
      {/* Grid overlay — light mode */}
      <div
        className="pointer-events-none fixed inset-0 z-0 dark:hidden auth-grid-mask"
        style={{
          backgroundImage:
            "linear-gradient(to right, rgba(0,0,0,0.04) 1px, transparent 1px), linear-gradient(to bottom, rgba(0,0,0,0.04) 1px, transparent 1px)",
          backgroundSize: "40px 40px",
        }}
        aria-hidden="true"
      />
      {/* Grid overlay — dark mode */}
      <div
        className="pointer-events-none fixed inset-0 z-0 hidden dark:block auth-grid-mask"
        style={{
          backgroundImage:
            "linear-gradient(to right, rgba(255,255,255,0.04) 1px, transparent 1px), linear-gradient(to bottom, rgba(255,255,255,0.04) 1px, transparent 1px)",
          backgroundSize: "40px 40px",
        }}
        aria-hidden="true"
      />

      {/* Theme toggle — top right */}
      <div className="absolute right-5 top-5 z-20">
        <ThemeToggle />
      </div>

      <main className="relative z-10 flex w-full flex-1">
        {/* Left: sign-in form */}
        <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col items-center justify-center px-5 py-8 sm:px-8 sm:py-10 md:items-start md:px-16 md:py-12 lg:px-24 xl:px-32">
          <div className="mb-8 w-full max-w-md sm:mb-12">
            <div className="flex items-center justify-center gap-3 md:justify-start">
              <LogoMark />
              <span className="text-xl font-bold uppercase tracking-tight text-foreground">
                SpaceScale
              </span>
            </div>
          </div>

          <div className="w-full max-w-md space-y-6 sm:space-y-8">
            <div className="text-center md:text-left">
              <h1 className="mb-2 text-2xl font-[200] tracking-tight text-foreground sm:mb-3 sm:text-3xl">
                Sign in
              </h1>
              <p className="text-sm font-light text-muted-foreground">
                A smarter deployment platform.
              </p>
            </div>

            <div className="space-y-3">
              {/* GitHub button */}
              <Button
                type="button"
                onClick={loginWithGithub}
                className={cn(
                  "group relative h-11 w-full overflow-hidden rounded-lg text-sm font-medium transition-all duration-200 sm:h-12",
                  // Light
                  "border border-border bg-card text-foreground shadow-sm hover:bg-muted",
                  // Dark override
                  "dark:border-white/10 dark:bg-white/[0.03] dark:text-white dark:shadow-[0_8px_32px_0_rgba(0,0,0,0.37)] dark:hover:bg-white/[0.08]",
                )}
                aria-label="Continue with GitHub"
              >
                <Github className="mr-3 h-5 w-5" aria-hidden="true" />
                <span>Continue with GitHub</span>
              </Button>

              {/* Google button */}
              <Button
                type="button"
                onClick={loginWithGoogle}
                className={cn(
                  "h-11 w-full rounded-lg text-sm transition-all duration-200 sm:h-12",
                  // Light
                  "border border-border bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground",
                  // Dark override
                  "dark:border-white/10 dark:text-white/90 dark:hover:bg-white/[0.06] dark:hover:text-white",
                )}
                aria-label="Continue with Google"
              >
                Continue with Google
              </Button>

              <div className="pt-6">
                <div className="flex justify-center gap-6 text-xs text-muted-foreground md:justify-start">
                  <a className="transition-colors hover:text-foreground" href={docsHref}>
                    Docs
                  </a>
                  <a className="transition-colors hover:text-foreground" href={articlesHref}>
                    Articles
                  </a>
                </div>
              </div>
            </div>
          </div>

          {/* Trusted logos strip */}
          <div className="mt-12 w-full max-w-md border-t border-border/50 pt-6 sm:mt-20 sm:pt-8">
            <p className="mb-6 text-center text-[10px] uppercase tracking-widest text-muted-foreground md:text-left">
              Trusted by developers at
            </p>
            <div className="flex flex-wrap items-center justify-center gap-x-6 gap-y-3 text-xs font-semibold text-foreground/60 sm:hidden">
              <span>twilio</span>
              <span>Alibaba.com</span>
              <span>Bridge</span>
              <span>Base 44</span>
              <span>CONTENT SQUARE</span>
            </div>
            <div className="auth-logo-strip-mask group relative hidden gap-8 overflow-hidden opacity-40 grayscale transition-all duration-700 hover:opacity-80 sm:flex">
              <TrustedRow />
              <TrustedRow ariaHidden />
            </div>
          </div>
        </div>

        {/* Right: feature preview panel */}
        <div className="relative hidden flex-1 border-l border-border/40 lg:flex">
          {/* Panel background */}
          <div className="absolute inset-0 bg-muted/20 dark:bg-[#0b0d14]/30" />

          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <div className="w-[80%] max-w-md animate-auth-float">
              <Card
                className={cn(
                  "relative overflow-hidden rounded-xl p-1",
                  // Light
                  "border border-border/60 bg-card shadow-[0_20px_60px_-15px_rgba(0,0,0,0.12),_0_4px_20px_-5px_rgba(0,0,0,0.06)]",
                  // Dark
                  "dark:border-white/[0.06] dark:bg-[rgba(15,23,42,0.6)] dark:backdrop-blur-[20px] dark:shadow-[0_25px_50px_-12px_rgba(0,0,0,0.5),_0_4px_20px_-5px_rgba(0,0,0,0.3)]",
                )}
              >
                {/* Indigo gradient line */}
                <div className="absolute left-0 top-0 h-[1px] w-full bg-gradient-to-r from-transparent via-primary to-transparent opacity-60" />

                <CardContent className="relative overflow-hidden rounded-lg bg-card p-8 pt-8 dark:bg-[#0f111a]">
                  <div className="mb-6 flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <span className="h-2 w-2 animate-pulse rounded-full bg-primary" />
                      <Badge
                        className={cn(
                          "border-primary/40 bg-primary/10 px-2 py-0 text-[10px] font-medium uppercase tracking-wider text-primary",
                          "hover:bg-primary/20",
                        )}
                      >
                        New Feature
                      </Badge>
                    </div>
                    <span className="font-mono text-xs text-muted-foreground">v2.4.0</span>
                  </div>

                  <h3 className="mb-3 text-xl font-light text-foreground">
                    Zero-Config Edge Functions
                  </h3>
                  <p className="mb-6 text-sm leading-relaxed text-muted-foreground">
                    Deploy serverless functions to 35+ regions instantly. No cold starts, automatic
                    scaling, and built-in observability from day one.
                  </p>

                  <div
                    className={cn(
                      "relative flex h-32 items-end justify-center overflow-hidden rounded border",
                      "border-border/50 bg-muted/40",
                      "dark:border-white/5 dark:bg-slate-900/50",
                    )}
                  >
                    <svg
                      className="h-full w-full text-primary/20"
                      preserveAspectRatio="none"
                      viewBox="0 0 200 100"
                    >
                      <title>Feature trend graph</title>
                      <path
                        d="M0,100 L0,80 C20,75 40,90 60,60 C80,30 100,40 120,20 C140,5 160,15 200,0 V100 Z"
                        fill="currentColor"
                      />
                      <path
                        d="M0,80 C20,75 40,90 60,60 C80,30 100,40 120,20 C140,5 160,15 200,0"
                        fill="none"
                        stroke="hsl(var(--primary))"
                        strokeWidth="2"
                      />
                    </svg>
                  </div>

                  <a
                    className="group mt-6 flex cursor-pointer items-center gap-1 text-xs text-muted-foreground/60 transition-colors hover:text-foreground pointer-events-auto"
                    href={articlesHref}
                  >
                    <span>Read the announcement</span>
                    <ArrowRight className="h-3.5 w-3.5 transition-transform group-hover:translate-x-1" />
                  </a>
                </CardContent>
              </Card>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
