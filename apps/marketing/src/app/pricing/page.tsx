import type { Metadata } from "next";
import { Badge } from "@spacescale/ui";
import { MarketingShell } from "@/components/marketing-shell";

export const metadata: Metadata = {
  title: "Pricing | SpaceScale",
  description:
    "Transparent pricing for builders and teams running on SpaceScale.",
};

type PricingPlan = {
  name: string;
  price: string;
  period: string;
  description: string;
  features: string[];
  featured?: boolean;
};

const plans: PricingPlan[] = [
  {
    name: "Starter",
    price: "$0",
    period: "/month",
    description: "For personal projects and quick experiments.",
    features: [
      "3 active services",
      "Community support",
      "Shared compute",
      "Basic logs and metrics",
    ],
  },
  {
    name: "Growth",
    price: "$49",
    period: "/month",
    description: "For growing products that need reliability and speed.",
    featured: true,
    features: [
      "25 active services",
      "Priority build queue",
      "Autoscaling",
      "Advanced metrics and alerts",
      "Email support",
    ],
  },
  {
    name: "Scale",
    price: "Custom",
    period: "",
    description: "For teams with enterprise workloads and compliance needs.",
    features: [
      "Unlimited services",
      "Dedicated infrastructure",
      "SAML/SSO",
      "Compliance controls",
      "Dedicated success engineer",
    ],
  },
];

export default function PricingPage() {
  return (
    <MarketingShell
      title="Simple pricing for every stage"
      description="Start free, scale when your traffic grows, and only pay for the resources you actually use."
    >
      <section className="grid gap-6 md:grid-cols-3">
        {plans.map((plan) => (
          <article
            key={plan.name}
            className={`rounded-2xl border p-6 backdrop-blur-sm ${
              plan.featured
                ? "border-indigo-400/50 bg-indigo-500/10 shadow-[0_0_40px_-16px_rgba(99,102,241,0.6)]"
                : "border-white/10 bg-white/[0.02]"
            }`}
          >
            <div className="mb-4 flex items-baseline justify-between">
              <h2 className="text-xl font-medium text-white">{plan.name}</h2>
              {plan.featured ? (
                <Badge variant="deploying" className="rounded-full border-indigo-400/40 bg-indigo-500/15 text-[10px] text-indigo-300">
                  Most Popular
                </Badge>
              ) : null}
            </div>
            <p className="mb-3 text-3xl font-[200] tracking-tight text-off-white">
              {plan.price}
              <span className="text-base font-light text-railway-muted">
                {plan.period}
              </span>
            </p>
            <p className="mb-5 text-sm text-railway-muted">
              {plan.description}
            </p>
            <ul className="space-y-2 text-sm text-railway-text">
              {plan.features.map((feature) => (
                <li key={feature} className="flex items-center gap-2">
                  <span className="size-1.5 rounded-full bg-indigo-400/90" />
                  {feature}
                </li>
              ))}
            </ul>
          </article>
        ))}
      </section>

      <section className="rounded-2xl border border-white/10 bg-white/[0.02] p-6">
        <h3 className="mb-2 text-lg font-medium text-white">
          Usage-based add-ons
        </h3>
        <p className="mb-4 text-sm text-railway-muted">
          Add storage, bandwidth, and premium observability as your product
          scales.
        </p>
        <div className="grid gap-3 text-sm text-railway-text md:grid-cols-2">
          <p>Bandwidth overage: $0.08 per GB</p>
          <p>Build minutes overage: $0.03 per minute</p>
          <p>Log retention (30 days): $9 / service</p>
          <p>Private networking: $15 / project</p>
        </div>
      </section>
    </MarketingShell>
  );
}
