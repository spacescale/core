"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Github } from "lucide-react";
import { Badge, Button, Card, CardContent, LogoMark } from "@spacescale/ui";
import { useAuth } from "@/lib/hooks";

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
        <div className="flex h-4 w-4 items-center justify-center rounded-full border border-white">
          <div className="h-1.5 w-1.5 rounded-full bg-white" />
        </div>
        <span className="text-sm font-bold tracking-tight text-white">twilio</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-sm font-bold italic text-white">Alibaba.com</span>
      </div>
      <div className="flex items-center gap-2">
        <div className="h-3 w-3 rounded-full border border-dashed border-white/60" />
        <span className="text-xs font-semibold text-white">Bridge</span>
      </div>
      <div className="flex items-center gap-2">
        <div className="relative h-4 w-4 overflow-hidden rounded-full bg-white/20">
          <div className="absolute bottom-0 h-1/2 w-full bg-white" />
        </div>
        <span className="text-xs font-bold text-white">Base 44</span>
      </div>
      <div className="origin-left scale-75 leading-none">
        <span className="block text-[8px] font-bold tracking-widest text-white">CONTENT</span>
        <span className="block text-[8px] font-bold tracking-widest text-white">SQUARE</span>
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
  const docsHref = `${marketingBaseUrl}/terms`;
  const articlesHref = `${marketingBaseUrl}/pricing`;

  useEffect(() => {
    if (isAuthenticated) {
      router.replace("/app");
    }
  }, [isAuthenticated, router]);

  if (isLoading || isAuthenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[#0f111a]">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-indigo-500 border-t-transparent" />
      </div>
    );
  }

  return (
    <div className="relative flex min-h-dvh flex-col overflow-x-hidden bg-[#0f111a] text-[#cacedb]">
      <div className="auth-grid-mask auth-grid-pattern bg-square-grid pointer-events-none fixed inset-0 z-0 opacity-[0.8]" />

      <main className="relative z-10 flex w-full flex-1">
        <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col items-center justify-center px-5 py-8 sm:px-8 sm:py-10 md:items-start md:px-16 md:py-12 lg:px-24 xl:px-32">
          <div className="mb-8 w-full max-w-md sm:mb-12">
            <div className="flex items-center justify-center gap-3 md:justify-start">
              <LogoMark />
              <span className="text-xl font-bold uppercase tracking-tight text-white">SpaceScale</span>
            </div>
          </div>

          <div className="w-full max-w-md space-y-6 sm:space-y-8">
            <div className="text-center md:text-left">
              <h1 className="mb-2 text-2xl font-[200] tracking-tight text-white sm:mb-3 sm:text-3xl">
                Sign in
              </h1>
              <p className="text-sm font-light text-[#6b7280]">A smarter deployment platform.</p>
            </div>

            <div className="space-y-3">
              <Button
                type="button"
                onClick={loginWithGithub}
                className="group relative h-11 w-full overflow-hidden rounded-lg border border-white/10 bg-white/[0.03] text-white shadow-[0_8px_32px_0_rgba(0,0,0,0.37)] transition-all duration-200 hover:bg-white/[0.08] sm:h-12"
                aria-label="Continue with GitHub"
              >
                <Github className="mr-3 h-5 w-5" aria-hidden="true" />
                <span className="text-sm font-medium">Continue with GitHub</span>
              </Button>

              <Button
                type="button"
                onClick={loginWithGoogle}
                className="h-11 w-full rounded-lg border border-white/10 bg-transparent text-white/90 transition-all duration-200 hover:bg-white/[0.06] hover:text-white sm:h-12"
                aria-label="Continue with Google"
              >
                Continue with Google
              </Button>

              <div className="pt-6">
                <div className="flex justify-center gap-6 text-xs text-[#6b7280] md:justify-start">
                  <a className="transition-colors hover:text-[#e2e4e9]" href={docsHref}>
                    Docs
                  </a>
                  <a className="transition-colors hover:text-[#e2e4e9]" href={articlesHref}>
                    Articles
                  </a>
                </div>
              </div>
            </div>
          </div>

          <div className="mt-12 w-full max-w-md border-t border-white/5 pt-6 sm:mt-20 sm:pt-8">
            <p className="mb-6 text-center text-[10px] uppercase tracking-widest text-[#6b7280] md:text-left">
              Trusted by developers at
            </p>
            <div className="flex flex-wrap items-center justify-center gap-x-6 gap-y-3 text-xs font-semibold text-white/70 sm:hidden">
              <span>twilio</span>
              <span>Alibaba.com</span>
              <span>Bridge</span>
              <span>Base 44</span>
              <span>CONTENT SQUARE</span>
            </div>
            <div className="auth-logo-strip-mask group relative hidden gap-8 overflow-hidden grayscale opacity-40 transition-all duration-700 hover:opacity-80 sm:flex">
              <TrustedRow />
              <TrustedRow ariaHidden />
            </div>
          </div>
        </div>

        <div className="relative hidden flex-1 border-l border-white/5 bg-[#0b0d14]/30 lg:flex">
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
            <div className="w-[80%] max-w-md animate-auth-float">
              <Card className="auth-glass-panel relative overflow-hidden rounded-xl border border-white/5 p-1 shadow-2xl">
                <div className="absolute left-0 top-0 h-1 w-full bg-gradient-to-r from-transparent via-indigo-500 to-transparent opacity-50" />
                <CardContent className="relative overflow-hidden rounded-lg bg-[#0f111a] p-8 pt-8">
                  <div className="mb-6 flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <span className="h-2 w-2 animate-pulse rounded-full bg-indigo-500" />
                      <Badge className="border-indigo-500/40 bg-indigo-500/10 px-2 py-0 text-[10px] font-medium uppercase tracking-wider text-indigo-300 hover:bg-indigo-500/20">
                        New Feature
                      </Badge>
                    </div>
                    <span className="font-mono text-xs text-[#6b7280]">v2.4.0</span>
                  </div>

                  <h3 className="mb-3 text-xl font-light text-white">Zero-Config Edge Functions</h3>
                  <p className="mb-6 text-sm leading-relaxed text-[#6b7280]">
                    Deploy serverless functions to 35+ regions instantly. No cold starts, automatic
                    scaling, and built-in observability from day one.
                  </p>

                  <div className="relative flex h-32 items-end justify-center overflow-hidden rounded border border-white/5 bg-slate-900/50">
                    <svg
                      className="h-full w-full text-indigo-500/20"
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
                        stroke="#6366f1"
                        strokeWidth="2"
                      />
                    </svg>
                  </div>

                  <a
                    className="group mt-6 flex cursor-pointer items-center gap-1 text-xs text-white/40 transition-colors hover:text-white"
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
