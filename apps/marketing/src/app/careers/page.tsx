import type { Metadata } from "next";
import { MarketingShell } from "@/components/marketing-shell";

export const metadata: Metadata = {
  title: "Careers | SpaceScale",
  description:
    "Join the SpaceScale team building modern cloud deployment tooling.",
};

export default function CareersPage() {
  return (
    <MarketingShell
      title="Careers at SpaceScale"
      description="We build infrastructure products that help teams ship fast and operate confidently."
    >
      <section className="rounded-2xl border border-amber-400/40 bg-amber-500/10 p-6 shadow-[0_0_40px_-18px_rgba(245,158,11,0.6)]">
        <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-amber-300">
          Hiring Status
        </p>
        <h2 className="mb-2 text-2xl font-[300] text-white">
          No open roles right now
        </h2>
        <p className="text-sm text-railway-text">
          We are not actively hiring at the moment. You can still send your
          resume to
          <a
            className="ml-1 text-amber-200 underline-offset-2 hover:underline"
            href="mailto:careers@spacescale.dev"
          >
            careers@spacescale.dev
          </a>
          and we will reach out when relevant roles open.
        </p>
      </section>

      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6">
        <h3 className="mb-3 text-lg font-medium text-white">What we value</h3>
        <ul className="space-y-2 text-sm text-railway-text">
          <li>Product ownership from idea to production</li>
          <li>Clear communication and kind collaboration</li>
          <li>Bias for simple, maintainable systems</li>
          <li>Healthy pace and sustainable shipping culture</li>
        </ul>
      </section>
    </MarketingShell>
  );
}
