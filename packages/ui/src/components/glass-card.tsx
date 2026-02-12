"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../lib/utils";

/**
 * GlassCard — A card with frosted-glass (glassmorphism) styling.
 *
 * Extracted from the SpaceScale dashboard designs where nearly every panel
 * uses a translucent backdrop-blur surface with subtle borders.
 *
 * @example
 * ```tsx
 * <GlassCard>
 *   <GlassCardHeader>
 *     <h3>Deployments</h3>
 *   </GlassCardHeader>
 *   <GlassCardContent>…</GlassCardContent>
 * </GlassCard>
 * ```
 */

const glassCardVariants = cva(
  "rounded-xl border transition-all duration-300 ease-[cubic-bezier(0.4,0,0.2,1)]",
  {
    variants: {
      variant: {
        default: "glass",
        elevated:
          "glass hover:shadow-[0_0_20px_var(--glow-primary)] hover:border-primary/20 hover:-translate-y-0.5",
        flat: "bg-card/50 border-border/50",
        interactive:
          "glass cursor-pointer hover:shadow-[0_0_20px_var(--glow-primary)] hover:border-primary/20 hover:-translate-y-1",
      },
      padding: {
        none: "",
        sm: "p-4",
        default: "p-6",
        lg: "p-8",
      },
    },
    defaultVariants: {
      variant: "default",
      padding: "default",
    },
  }
);

export interface GlassCardProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof glassCardVariants> {}

const GlassCard = React.forwardRef<HTMLDivElement, GlassCardProps>(
  ({ className, variant, padding, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(glassCardVariants({ variant, padding, className }))}
      {...props}
    />
  )
);
GlassCard.displayName = "GlassCard";

const GlassCardHeader = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn("flex flex-col space-y-1.5 pb-4", className)}
    {...props}
  />
));
GlassCardHeader.displayName = "GlassCardHeader";

const GlassCardContent = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn("", className)} {...props} />
));
GlassCardContent.displayName = "GlassCardContent";

const GlassCardFooter = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      "flex items-center pt-4 border-t border-border/50",
      className
    )}
    {...props}
  />
));
GlassCardFooter.displayName = "GlassCardFooter";

export {
  GlassCard,
  GlassCardHeader,
  GlassCardContent,
  GlassCardFooter,
  glassCardVariants,
};
