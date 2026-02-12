import type { Metadata } from "next";
import { MarketingShell } from "@/components/marketing-shell";

export const metadata: Metadata = {
  title: "Privacy Policy | SpaceScale",
  description: "Learn how SpaceScale collects, uses, and protects your data.",
};

const effectiveDate = "February 12, 2026";

export default function PrivacyPage() {
  return (
    <MarketingShell
      title="Privacy Policy"
      description="This policy explains how SpaceScale handles personal data when you use our services."
    >
      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6 text-sm text-railway-text">
        <p className="mb-4 text-railway-muted">
          Effective date: {effectiveDate}
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">
          Information we collect
        </h2>
        <p className="mb-4">
          We collect account information, usage metadata, billing details, and
          support communications.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">
          How we use information
        </h2>
        <p className="mb-4">
          We use data to provide the service, secure the platform, improve
          reliability, and support customers.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">Data sharing</h2>
        <p className="mb-4">
          We do not sell personal data. We share data only with service
          providers needed to run SpaceScale and when required by law.
        </p>

        <h2 className="mb-2 text-lg font-medium text-white">Your choices</h2>
        <p>
          You can request access, correction, or deletion of personal data by
          contacting
          <a
            className="ml-1 text-indigo-300 underline-offset-2 hover:underline"
            href="mailto:privacy@spacescale.dev"
          >
            privacy@spacescale.dev
          </a>
          .
        </p>
      </section>
    </MarketingShell>
  );
}
