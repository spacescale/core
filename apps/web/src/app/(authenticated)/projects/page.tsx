"use client";

import Link from "next/link";
import { useState } from "react";
import { ArrowRight, ChevronDown, Grid2X2, List, Plus, Settings, Star } from "lucide-react";
import { SearchInput } from "@spacescale/ui";
import { cn } from "@/lib/utils";

// ─── Types & Mock Data ────────────────────────────────────────────────────────

type ProjectStatus = "healthy" | "warning" | "critical";

interface Project {
  id: string;
  name: string;
  status: ProjectStatus;
  resources: number;
  updatedAt: string;
  starred: boolean;
}

const MOCK_PROJECTS: Project[] = [
  { id: "silent-mountain", name: "silent-mountain", status: "healthy",  resources: 7,  updatedAt: "15m", starred: true  },
  { id: "crimson-tide",    name: "crimson-tide",    status: "warning",  resources: 12, updatedAt: "2h",  starred: false },
  { id: "neon-vector",     name: "neon-vector",     status: "critical", resources: 4,  updatedAt: "5m",  starred: false },
  { id: "oceanic-depth",   name: "oceanic-depth",   status: "healthy",  resources: 23, updatedAt: "1d",  starred: false },
  { id: "lunar-orbit",     name: "lunar-orbit",     status: "warning",  resources: 9,  updatedAt: "4h",  starred: false },
  { id: "void-runner",     name: "void-runner",     status: "healthy",  resources: 15, updatedAt: "8m",  starred: false },
];

// ─── Status Config ────────────────────────────────────────────────────────────

const statusConfig: Record<ProjectStatus, { label: string; dotClass: string; textClass: string }> = {
  healthy:  { label: "Healthy",  dotClass: "bg-success",     textClass: "text-success"     },
  warning:  { label: "Warning",  dotClass: "bg-warning",     textClass: "text-warning"     },
  critical: { label: "Critical", dotClass: "bg-destructive", textClass: "text-destructive" },
};

// ─── Status Dot (pulse ring) ──────────────────────────────────────────────────

function StatusDot({ status }: { status: ProjectStatus }) {
  const { dotClass } = statusConfig[status];
  return (
    <div className="relative w-1.5 h-1.5 flex-shrink-0">
      <span className={cn("absolute inset-0 rounded-full animate-status-ping opacity-50", dotClass)} />
      <span className={cn("relative w-1.5 h-1.5 rounded-full block", dotClass)} />
    </div>
  );
}

// ─── Project Icon ─────────────────────────────────────────────────────────────

function ProjectIcon({ size = "md" }: { size?: "sm" | "md" }) {
  return (
    <svg
      className={size === "md" ? "w-5 h-5" : "w-4 h-4"}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9"
      />
    </svg>
  );
}

// ─── Grid Card ────────────────────────────────────────────────────────────────

function ProjectCard({
  project,
  onToggleStar,
}: {
  project: Project;
  onToggleStar: (id: string) => void;
}) {
  const { label } = statusConfig[project.status];

  return (
    <Link
      href={`/projects/${project.id}`}
      className={cn(
        "group relative h-48 rounded-xl hairline-border p-6",
        "flex flex-col items-start justify-between text-left",
        "overflow-hidden cursor-pointer transition-all duration-300",
        // Light
        "bg-card border-border/60",
        "hover:border-border hover:shadow-[0_8px_30px_-8px_rgba(0,0,0,0.1)] hover:-translate-y-px",
        // Dark
        "dark:bg-card dark:border-white/[0.08]",
        "dark:hover:bg-card dark:hover:border-white/[0.25]",
        "dark:hover:shadow-[0_0_24px_-4px_rgba(99,102,241,0.12)]",
      )}
    >
      {/* Top row */}
      <div className="relative z-10 w-full flex items-start justify-between">
        <div className={cn(
          "w-10 h-10 rounded-lg flex items-center justify-center",
          "bg-muted/60 dark:bg-white/[0.03] text-muted-foreground",
        )}>
          <ProjectIcon size="md" />
        </div>

        <button
          type="button"
          onClick={(e) => e.preventDefault()}
          aria-label="Project settings"
          className={cn(
            "absolute top-0 right-0 p-1 rounded transition-opacity duration-300",
            "opacity-0 group-hover:opacity-100",
            "text-muted-foreground hover:text-foreground",
          )}
        >
          <Settings className="h-[15px] w-[15px]" strokeWidth={1.5} />
        </button>

        <div className="flex items-center gap-2 transition-opacity duration-300 group-hover:opacity-0">
          <span className="text-[10px] text-muted-foreground font-mono tabular-nums">
            {project.updatedAt}
          </span>
          <button
            type="button"
            onClick={(e) => { e.preventDefault(); onToggleStar(project.id); }}
            aria-label={project.starred ? "Unstar project" : "Star project"}
            className="leading-none"
          >
            <Star
              className={cn(
                "h-3.5 w-3.5 transition-colors",
                project.starred
                  ? "fill-current text-primary"
                  : "text-muted-foreground/30 hover:text-muted-foreground",
              )}
              strokeWidth={1.5}
            />
          </button>
        </div>
      </div>

      {/* Bottom */}
      <div className="relative z-10 w-full mt-auto">
        <div className="flex items-center gap-2 mb-2">
          <StatusDot status={project.status} />
          <span
            className={cn(
              "text-[10px] uppercase tracking-widest font-medium transition-colors duration-300 text-foreground/70",
              project.status === "healthy"  && "group-hover:text-success",
              project.status === "warning"  && "group-hover:text-warning",
              project.status === "critical" && "group-hover:text-destructive",
            )}
          >
            {label}
          </span>
        </div>

        <h3 className="text-[15px] font-[200] text-foreground mb-1 tracking-wide">
          {project.name}
        </h3>

        <div className="relative h-[18px]">
          <p className="text-[11px] text-muted-foreground font-[200] tracking-wider absolute inset-0 transition-opacity duration-300 group-hover:opacity-0">
            {project.resources} Resources
          </p>
          <div className="absolute bottom-[-2px] right-0 flex items-center gap-1 opacity-0 group-hover:opacity-100 translate-y-1 group-hover:translate-y-0 transition-all duration-300">
            <span className="text-[10px] text-primary font-mono tracking-wider">ENTER</span>
            <ArrowRight className="h-3 w-3 text-primary" strokeWidth={2} />
          </div>
        </div>
      </div>
    </Link>
  );
}

// ─── List Row ─────────────────────────────────────────────────────────────────

function ProjectRow({
  project,
  onToggleStar,
}: {
  project: Project;
  onToggleStar: (id: string) => void;
}) {
  const { label } = statusConfig[project.status];

  return (
    <div
      className={cn(
        "group relative flex items-center justify-between px-6 py-4 rounded-lg transition-all duration-200",
        "bg-card border border-border/60 dark:border-white/[0.08]",
        "hover:bg-muted/40 dark:hover:bg-muted/20",
        "overflow-hidden",
      )}
    >
      <div className="absolute left-0 top-0 bottom-0 w-[2px] bg-gradient-to-b from-indigo-500 to-purple-500 opacity-0 group-hover:opacity-100 transition-opacity duration-200" />

      <div className="flex items-center gap-5 flex-1 min-w-0">
        <div className={cn(
          "w-8 h-8 rounded flex items-center justify-center flex-shrink-0",
          "bg-muted/50 dark:bg-white/[0.05] text-muted-foreground",
        )}>
          <ProjectIcon size="sm" />
        </div>

        <Link
          href={`/projects/${project.id}`}
          className="text-[14px] font-[200] text-foreground tracking-wide hover:text-primary transition-colors min-w-[180px]"
          onClick={(e) => e.stopPropagation()}
        >
          {project.name}
        </Link>

        <div className="flex items-center gap-2 min-w-[110px]">
          <StatusDot status={project.status} />
          <span className={cn("text-[11px] font-medium tracking-wide", statusConfig[project.status].textClass)}>
            {label}
          </span>
        </div>

        <div className="hidden sm:block text-[12px] text-muted-foreground font-[200] tracking-wide min-w-[100px]">
          {project.resources} Resources
        </div>
      </div>

      <div className="flex items-center gap-5 pr-2 flex-shrink-0">
        <span className="hidden md:block text-[11px] font-[200] text-muted-foreground font-mono tabular-nums group-hover:text-foreground/50 transition-colors">
          {project.updatedAt}
        </span>

        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 translate-x-2 group-hover:translate-x-0 transition-all duration-200">
            <button
              type="button"
              className={cn(
                "p-1.5 rounded transition-colors",
                "text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/10",
              )}
              aria-label="Project settings"
            >
              <Settings className="h-[15px] w-[15px]" strokeWidth={1.5} />
            </button>
            <Link
              href={`/projects/${project.id}`}
              className={cn(
                "p-1.5 rounded transition-colors",
                "text-primary hover:text-primary/80 hover:bg-primary/10",
              )}
              aria-label="Open project"
              onClick={(e) => e.stopPropagation()}
            >
              <ArrowRight className="h-[15px] w-[15px]" strokeWidth={1.5} />
            </Link>
          </div>

          <div className="w-px h-4 bg-border/60 dark:bg-white/10" />

          <button
            type="button"
            onClick={() => onToggleStar(project.id)}
            aria-label={project.starred ? "Unstar project" : "Star project"}
          >
            <Star
              className={cn(
                "h-4 w-4 transition-colors",
                project.starred
                  ? "fill-current text-primary"
                  : "text-muted-foreground/25 hover:text-muted-foreground",
              )}
              strokeWidth={1.5}
            />
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── Projects Page ────────────────────────────────────────────────────────────

type SortKey = "recent" | "name" | "status";

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: "recent", label: "Recent" },
  { value: "name",   label: "Name"   },
  { value: "status", label: "Status" },
];

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[]>(MOCK_PROJECTS);
  const [search, setSearch] = useState("");
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [sortBy, setSortBy] = useState<SortKey>("recent");
  const [sortOpen, setSortOpen] = useState(false);

  const filtered = projects
    .filter((p) => p.name.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => {
      if (sortBy === "name")   return a.name.localeCompare(b.name);
      if (sortBy === "status") return a.status.localeCompare(b.status);
      return 0;
    });

  function toggleStar(id: string) {
    setProjects((prev) =>
      prev.map((p) => (p.id === id ? { ...p, starred: !p.starred } : p)),
    );
  }

  return (
    <div className="animate-view-in">
      {/* Toolbar */}
      <div className="mb-8 flex items-center justify-between gap-4 flex-wrap">
        <div className="flex items-center gap-3">
          <SearchInput
            placeholder="Search projects…"
            value={search}
            onValueChange={setSearch}
            className="max-w-xs"
          />

          {/* Sort by */}
          <div className="relative">
            <button
              type="button"
              onClick={() => setSortOpen((o) => !o)}
              className={cn(
                "flex items-center gap-2 h-[38px] pl-3 pr-2.5 rounded-lg text-xs font-medium transition-all",
                "border border-border/60 dark:border-white/[0.08]",
                "bg-muted/40 dark:bg-white/[0.03]",
                "text-muted-foreground hover:text-foreground",
              )}
            >
              <span className="text-[10px] tracking-widest uppercase text-muted-foreground/60 mr-0.5">
                Sort by
              </span>
              <span className="text-foreground">
                {SORT_OPTIONS.find((o) => o.value === sortBy)?.label}
              </span>
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" strokeWidth={1.5} />
            </button>

            {sortOpen && (
              <div
                className={cn(
                  "absolute top-full left-0 mt-1 z-20 w-36 rounded-lg py-1 shadow-lg",
                  "bg-card dark:bg-[#181b26]",
                  "border border-border/60 dark:border-white/[0.08]",
                )}
              >
                {SORT_OPTIONS.map((opt) => (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => { setSortBy(opt.value); setSortOpen(false); }}
                    className={cn(
                      "w-full text-left px-3 py-1.5 text-xs transition-colors",
                      sortBy === opt.value
                        ? "text-primary"
                        : "text-muted-foreground hover:text-foreground hover:bg-muted/50 dark:hover:bg-white/[0.04]",
                    )}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* View toggle */}
          <div
            className={cn(
              "flex items-center h-[38px] rounded-lg p-1 gap-0.5",
              "bg-muted/40 dark:bg-white/[0.03]",
              "border border-border/60 dark:border-white/[0.08]",
            )}
          >
            <button
              type="button"
              aria-label="Grid view"
              aria-pressed={viewMode === "grid"}
              onClick={() => setViewMode("grid")}
              className={cn(
                "p-1.5 rounded transition-colors",
                viewMode === "grid"
                  ? "bg-background dark:bg-white/[0.08] text-primary shadow-sm"
                  : "text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/5",
              )}
            >
              <Grid2X2 className="h-[15px] w-[15px]" strokeWidth={1.5} />
            </button>
            <div className="w-px h-4 bg-border/60 dark:bg-white/[0.08]" />
            <button
              type="button"
              aria-label="List view"
              aria-pressed={viewMode === "list"}
              onClick={() => setViewMode("list")}
              className={cn(
                "p-1.5 rounded transition-colors",
                viewMode === "list"
                  ? "bg-background dark:bg-white/[0.08] text-primary shadow-sm"
                  : "text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/5",
              )}
            >
              <List className="h-[15px] w-[15px]" strokeWidth={1.5} />
            </button>
          </div>
        </div>

        <Link
          href="/projects/new"
          className={cn(
            "flex items-center gap-2 pl-3 pr-4 py-2 h-[38px] rounded-lg text-sm font-[450] transition-all",
            "border border-border/60 dark:border-white/[0.08]",
            "bg-card/60 dark:bg-white/[0.03]",
            "text-foreground hover:text-primary hover:border-primary/30",
            "dark:hover:border-primary/30 dark:hover:bg-white/[0.06]",
          )}
        >
          <Plus className="h-4 w-4 text-primary" strokeWidth={2} />
          <span>New Project</span>
        </Link>
      </div>

      {/* Empty state */}
      {filtered.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 gap-3">
          <p className="text-muted-foreground text-sm">No projects match your search.</p>
          <button
            type="button"
            onClick={() => setSearch("")}
            className="text-xs text-primary hover:underline transition-colors"
          >
            Clear search
          </button>
        </div>
      )}

      {/* Grid view */}
      {filtered.length > 0 && viewMode === "grid" && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5">
          {filtered.map((project) => (
            <ProjectCard key={project.id} project={project} onToggleStar={toggleStar} />
          ))}
        </div>
      )}

      {/* List view */}
      {filtered.length > 0 && viewMode === "list" && (
        <div className="flex flex-col gap-3">
          {filtered.map((project) => (
            <ProjectRow key={project.id} project={project} onToggleStar={toggleStar} />
          ))}
        </div>
      )}
    </div>
  );
}
