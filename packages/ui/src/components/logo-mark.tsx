import * as React from "react";
import { cn } from "../lib/utils";

const columns = [1, 2, 2, 3];

export interface LogoMarkProps extends React.HTMLAttributes<HTMLDivElement> {
  blockClassName?: string;
}

export function LogoMark({
  className,
  blockClassName,
  ...props
}: LogoMarkProps) {
  return (
    <div
      className={cn("flex h-7 items-end gap-[3px]", className)}
      aria-hidden="true"
      {...props}
    >
      {columns.map((count, columnIndex) => (
        <div
          key={`column-${columnIndex}`}
          className="flex flex-col justify-end gap-[3px]"
        >
          {Array.from({ length: count }).map((_, blockIndex) => (
            <div
              key={`block-${columnIndex}-${blockIndex}`}
              className={cn(
                "h-1.5 w-1.5 rounded-[1px] bg-white",
                blockClassName,
              )}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
