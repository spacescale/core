import type { Metadata } from "next";
import { MarketingShell } from "@/components/marketing-shell";

export const metadata: Metadata = {
  title: "Terms of Service | SpaceScale",
  description: "Terms and conditions for using SpaceScale services.",
};

const effectiveDate = "February 12, 2026";

export default function TermsPage() {
  return (
    <MarketingShell
      title="Terms of Service"
      description="These terms govern your use of SpaceScale products and services."
    >
      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6 text-sm text-railway-text">
        <p className="mb-4 text-railway-muted">
          Effective date: {effectiveDate}
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">Use of service</h2>
        <p className="mb-4">
          You agree to use SpaceScale lawfully and in compliance with these
          terms and applicable regulations.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">
          Accounts and security
        </h2>
        <p className="mb-4">
          You are responsible for account credentials and activity under your
          account, including API key usage.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">
          Billing and refunds
        </h2>
        <p className="mb-4">
          Paid plans are billed according to selected pricing. Usage overages
          are billed in arrears. Fees are non-refundable unless required by law.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">
          Service availability
        </h2>
        <p className="mb-4">
          We aim for high availability but do not guarantee uninterrupted
          service. Maintenance and incidents can cause temporary downtime.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">Contact</h2>
        <p>
          Questions about these terms can be sent to
          <a
            className="ml-1 text-indigo-300 underline-offset-2 hover:underline"
            href="mailto:legal@spacescale.dev"
          >
            legal@spacescale.dev
          </a>
          .
        </p>
      </section>
    </MarketingShell>
  );
}
