"use client";

import * as React from "react";
import { cn } from "../lib/utils";

/**
 * Sparkline — A minimal inline SVG chart for showing trends in compact spaces.
 *
 * Found on the SpaceScale dashboard overview and application metrics cards,
 * rendered as small line graphs inside stat cards.
 *
 * @example
 * ```tsx
 * <Sparkline data={[10, 25, 18, 40, 35, 55, 48]} />
 * <Sparkline data={cpuHistory} color="green" fill />
 * ```
 */

export interface SparklineProps extends Omit<React.SVGAttributes<SVGSVGElement>, "fill"> {
  data: number[];
  color?: "primary" | "green" | "red" | "amber" | "blue" | "purple";
  fill?: boolean;
  strokeWidth?: number;
}

const colorMap = {
  primary: { stroke: "hsl(var(--primary))", fill: "hsl(var(--primary) / 0.1)" },
  green: { stroke: "#10b981", fill: "rgba(16, 185, 129, 0.1)" },
  red: { stroke: "#ef4444", fill: "rgba(239, 68, 68, 0.1)" },
  amber: { stroke: "#f59e0b", fill: "rgba(245, 158, 11, 0.1)" },
  blue: { stroke: "#6366f1", fill: "rgba(99, 102, 241, 0.1)" },
  purple: { stroke: "#8b5cf6", fill: "rgba(139, 92, 246, 0.1)" },
};

const Sparkline = React.forwardRef<SVGSVGElement, SparklineProps>(
  (
    {
      className,
      data,
      color = "primary",
      fill: showFill = false,
      strokeWidth = 1.5,
      ...props
    },
    ref
  ) => {
    if (data.length < 2) return null;

    const width = 100;
    const height = 32;
    const padding = 1;

    const min = Math.min(...data);
    const max = Math.max(...data);
    const range = max - min || 1;

    const points = data.map((value, index) => ({
      x: padding + (index / (data.length - 1)) * (width - padding * 2),
      y:
        padding +
        (1 - (value - min) / range) * (height - padding * 2),
    }));

    const linePath = points
      .map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`)
      .join(" ");

    const fillPath = `${linePath} L ${points[points.length - 1].x} ${height} L ${points[0].x} ${height} Z`;

    const colors = colorMap[color];

    return (
      <svg
        ref={ref}
        viewBox={`0 0 ${width} ${height}`}
        className={cn("h-8 w-full", className)}
        preserveAspectRatio="none"
        {...props}
      >
        {showFill && (
          <path d={fillPath} fill={colors.fill} />
        )}
        <path
          d={linePath}
          fill="none"
          stroke={colors.stroke}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
    );
  }
);
Sparkline.displayName = "Sparkline";

export { Sparkline };
