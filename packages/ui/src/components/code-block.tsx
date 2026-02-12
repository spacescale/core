"use client";

import * as React from "react";
import { Copy, Check } from "lucide-react";
import { cn } from "../lib/utils";

/**
 * CodeBlock — A terminal/code display with optional copy button and line numbers.
 *
 * Used across SpaceScale for deployment logs, build output, terminal views,
 * and code snippets.
 *
 * @example
 * ```tsx
 * <CodeBlock code="npm install" language="bash" />
 * <CodeBlock code={logs} title="Build Output" showLineNumbers />
 * ```
 */

export interface CodeBlockProps extends React.HTMLAttributes<HTMLDivElement> {
  code: string;
  language?: string;
  title?: string;
  showLineNumbers?: boolean;
  copyable?: boolean;
  maxHeight?: string;
}

const CodeBlock = React.forwardRef<HTMLDivElement, CodeBlockProps>(
  (
    {
      className,
      code,
      language,
      title,
      showLineNumbers = false,
      copyable = true,
      maxHeight = "400px",
      ...props
    },
    ref
  ) => {
    const [copied, setCopied] = React.useState(false);

    const handleCopy = async () => {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    };

    const lines = code.split("\n");

    return (
      <div
        ref={ref}
        className={cn(
          "rounded-lg border bg-[hsl(222.2,84%,3%)] text-[hsl(210,40%,92%)] overflow-hidden",
          className
        )}
        {...props}
      >
        {(title || copyable) && (
          <div className="flex items-center justify-between border-b border-white/10 px-4 py-2">
            <div className="flex items-center gap-2">
              {title && (
                <span className="text-xs font-medium text-white/60">
                  {title}
                </span>
              )}
              {language && (
                <span className="rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-mono uppercase tracking-wider text-white/40">
                  {language}
                </span>
              )}
            </div>
            {copyable && (
              <button
                onClick={handleCopy}
                className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs text-white/40 transition-colors hover:bg-white/10 hover:text-white/70"
                aria-label="Copy code"
              >
                {copied ? (
                  <>
                    <Check className="h-3 w-3" />
                    <span>Copied</span>
                  </>
                ) : (
                  <>
                    <Copy className="h-3 w-3" />
                    <span>Copy</span>
                  </>
                )}
              </button>
            )}
          </div>
        )}
        <div
          className="overflow-auto scrollbar-thin"
          style={{ maxHeight }}
        >
          <pre className="p-4 text-sm leading-relaxed">
            <code className="font-mono text-[13px]">
              {lines.map((line, i) => (
                <div key={i} className="flex">
                  {showLineNumbers && (
                    <span className="mr-4 inline-block w-8 select-none text-right text-white/20 tabular-nums">
                      {i + 1}
                    </span>
                  )}
                  <span className="flex-1">{line || "\u00A0"}</span>
                </div>
              ))}
            </code>
          </pre>
        </div>
      </div>
    );
  }
);
CodeBlock.displayName = "CodeBlock";

export { CodeBlock };
