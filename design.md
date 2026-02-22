# SpaceScale Design System

> Feed this to any design/UI agent before building or modifying UI. All values are authoritative — do not invent alternatives.

---

## Brand

**Product**: SpaceScale — a PaaS (Platform as a Service) dashboard.
**Aesthetic**: Shell-terminal minimalism. Think Railway.app meets a command-line. Clean, precise, slightly cold. Not corporate, not playful.
**Voice in UI**: lowercase labels, monospace paths, uppercase tracking for status/badge text.

---

## Fonts

| Role | Font | CSS Variable | Tailwind Class |
|------|------|-------------|----------------|
| Display (body, headings, UI) | Manrope | `--font-manrope` | `font-display` |
| Monospace (paths, IDs, code, timestamps) | JetBrains Mono | `--font-jetbrains-mono` | `font-mono` |

**Weights in use**: 200 (light body), 300 (labels), 400 (default), 450 (medium CTA), bold only for badge text.
**Tracking**: `.tracking-wider` / `.tracking-widest` for uppercase labels. `.tracking-wide` for project names.
**Never use**: Inter, Roboto, system-ui, or any default sans-serif.

---

## Color Tokens

All colors are CSS custom properties consumed via Tailwind. Always use the semantic token — never hardcode hex.

### Light Mode (`:root`) — Alabaster palette

| Token | HSL | Hex | Usage |
|-------|-----|-----|-------|
| `--background` | 210 17% 98% | `#F8F9FA` | Page background |
| `--foreground` | 220 39% 17% | `#1F2937` | Body text, headings |
| `--card` | 0 0% 100% | `#FFFFFF` | Card surfaces (opaque) |
| `--primary` | 244 76% 59% | `#4F46E5` | CTA, active states, accents |
| `--muted` | 210 17% 96% | `#F3F4F6` | Subtle backgrounds, icon containers |
| `--muted-foreground` | 220 9% 46% | `#6B7280` | Secondary text, placeholders |
| `--border` | 220 13% 91% | `#E2E4E8` | Hairline borders |
| `--success` | 160 84% 39% | `#10B981` | Healthy status |
| `--warning` | 38 92% 50% | `#F59E0B` | Warning status |
| `--destructive` | 0 84% 60% | `#EF4444` | Critical / error |

### Dark Mode (`.dark`) — Railway palette

| Token | HSL | Hex | Usage |
|-------|-----|-----|-------|
| `--background` | 228 31% 9% | `#0f111a` | Page background |
| `--foreground` | 222 20% 83% | `#cacedb` | Body text |
| `--card` | 228 26% 12% | `#181b26` | Card surfaces (opaque) |
| `--primary` | 239 84% 67% | `#6366f1` | CTA, active states, indigo accents |
| `--muted` | 228 24% 18% | `#232836` | Subtle backgrounds |
| `--muted-foreground` | 220 9% 46% | `#6B7280` | Secondary text (same as light) |
| `--border` | 228 24% 18% | `#232836` | Borders |
| `--success` | 160 84% 39% | `#10B981` | Healthy (same as light) |
| `--warning` | 38 92% 50% | `#F59E0B` | Warning (same as light) |
| `--destructive` | 0 63% 45% | `#C53030` | Critical (darker in dark mode) |

### Glow tokens (available as CSS vars)

```css
--glow-primary   /* light: rgba(79,70,229,0.15)   dark: rgba(99,102,241,0.25) */
--glow-success   /* light: rgba(16,185,129,0.15)   dark: rgba(16,185,129,0.2) */
--glow-warning   /* light: rgba(245,158,11,0.15)   dark: rgba(245,158,11,0.2) */
--glow-destructive /* light: rgba(239,68,68,0.15)  dark: rgba(239,68,68,0.2) */
```

---

## Layout Shell

### Structure
```
┌──────────────────────────────────────────────┐  h-14  z-50  Header (fixed)
├──────────────────────────────────────────────┤  h-9   z-40  Subheader/breadcrumb (fixed)
│  Sidebar (fixed, w-64)  │  Main content       │              pt-[92px] total offset
│                         │  p-6 lg:p-8         │
└─────────────────────────┴─────────────────────┘
```

### Header (`h-14`, fixed, `z-50`)
- Light: `bg-[rgba(248,249,250,0.85)] backdrop-blur-[12px] border-b border-black/[0.06]`
- Dark: `bg-[rgba(15,17,26,0.7)] backdrop-blur-[12px] border-b border-white/[0.05]`
- Contents: Logo + "SpaceScale" (left) | PRO PLAN badge + Bell + ThemeToggle + Avatar (right)
- Logo text: `text-sm font-[200] tracking-[0.2em] uppercase`
- Avatar: `bg-gradient-to-tr from-primary to-purple-500` gradient, `h-8 w-8 rounded-full`

### Subheader (`h-9`, fixed, `top-14`, `z-40`)
- Font: `font-mono text-[12px]`
- Light: `bg-[rgba(248,249,250,0.65)] backdrop-blur-[8px] border-b border-black/[0.06]`
- Dark: `bg-background border-b border-white/[0.05]` (no blur)
- Pattern: `user@workspace-name % projects / [segment]`
- Active breadcrumb segment: `text-primary bg-primary/10 border border-primary/20`
- Parent segments: clickable `<Link>` with `text-muted-foreground hover:text-foreground`

### Sidebar (`w-64`, fixed left, full height)
- Light border-right: `border-r border-border/60`
- Dark border-right: `border-r border-white/[0.05]`
- Workspace indicator: `w-2 h-2 rounded-full bg-primary shadow-[0_0_12px_var(--glow-primary)]`
- Section label: `text-[10px] uppercase tracking-widest text-muted-foreground`
- Nav item active (light): `bg-white border border-gray-200 shadow-sm text-foreground`
- Nav item active (dark): `bg-white/[0.04] border border-white/[0.05] text-white`
- Nav item inactive: `text-muted-foreground hover:text-foreground hover:bg-muted/50`

---

## Component Patterns

### Cards (opaque — never transparent)

**Grid card** (`ProjectCard`):
- Light: `bg-card border border-border/60`
- Dark: `dark:bg-card dark:border-white/[0.08]`
- Hover light: `hover:border-border hover:shadow-[0_8px_30px_-8px_rgba(0,0,0,0.1)] hover:-translate-y-px`
- Hover dark: `dark:hover:border-white/[0.25] dark:hover:shadow-[0_0_24px_-4px_rgba(99,102,241,0.12)]`
- Border radius: `rounded-xl`
- Use `hairline-border` utility for retina-sharp borders

**List row** (`ProjectRow`):
- Each row is its own individual card: `rounded-lg bg-card border border-border/60 dark:border-white/[0.08]`
- Container: `flex flex-col gap-3` (gap between cards, no shared wrapper)
- Left-edge hover indicator: `absolute left-0 top-0 bottom-0 w-[2px] bg-gradient-to-b from-indigo-500 to-purple-500 opacity-0 group-hover:opacity-100`
- Hover: `hover:bg-muted/40 dark:hover:bg-muted/20`

### Status indicators
```tsx
// Pulse ring + solid dot
<div className="relative w-1.5 h-1.5">
  <span className="absolute inset-0 rounded-full animate-status-ping opacity-50 bg-success" />
  <span className="relative w-1.5 h-1.5 rounded-full block bg-success" />
</div>
```
Status label: `text-[10px] uppercase tracking-widest font-medium`

### PRO PLAN badge
`border border-primary/20 bg-primary/10 text-primary text-[10px] font-bold tracking-wider`

### Buttons (standard ghost icon)
`h-8 w-8 rounded-full text-muted-foreground hover:text-foreground hover:bg-black/5 dark:hover:bg-white/10`

### Sort / filter dropdowns
```
trigger: h-[38px] border border-border/60 dark:border-white/[0.08] bg-muted/40 dark:bg-white/[0.03]
label prefix: text-[10px] tracking-widest uppercase text-muted-foreground/60
dropdown panel: bg-card dark:bg-[#181b26] border border-border/60 dark:border-white/[0.08] rounded-lg shadow-lg
```

---

## Utility Classes

| Class | Purpose |
|-------|---------|
| `hairline-border` | 1px border, 0.5px on retina (`min-resolution: 2dppx`) |
| `bg-app-grid` | 40×40px dot grid, adapts light/dark automatically |
| `animate-view-in` | Page enter: fade + translateY(10px→0), 0.4s ease-out |
| `animate-status-ping` | Status dot pulse ring: scale(2) + fade out, 1s infinite |
| `animate-fade-in` | Simple opacity 0→1, 0.3s |
| `scrollbar-thin` | Thin styled scrollbar |
| `glass-header-light` | Header glass (light) — use only for header/subheader |
| `glass-header-dark` | Header glass (dark) — use only for header/subheader |
| `auth-glass-panel` | Login page glass panel (dark only, semi-transparent) |

---

## Glass Morphism

Only used in **header, subheader, and login page**. Content cards are **opaque** (`bg-card`).

| Surface | Light | Dark |
|---------|-------|------|
| Header | `rgba(248,249,250,0.85)` + blur 12px | `rgba(15,17,26,0.70)` + blur 12px |
| Subheader | `rgba(248,249,250,0.65)` + blur 8px | `bg-background` (no blur) |
| Login panel | n/a | `rgba(15,23,42,0.6)` + blur 20px |

**Rule**: Never add `backdrop-blur` or `bg-*/opacity` to content area cards. Cards use solid `bg-card`.

---

## Interaction Patterns

- **Hover lift**: Cards use `hover:-translate-y-px` (1px only — subtle)
- **Reveal on hover**: Actions/settings icons use `opacity-0 group-hover:opacity-100 translate-x-2 group-hover:translate-x-0 transition-all duration-200`
- **Active nav**: Border + slight background, not a filled pill
- **Transition duration**: `duration-200` for most UI, `duration-300` for cards
- **Page entry**: Always wrap page root in `<div className="animate-view-in">`

---

## Dark Mode Rules

- `darkMode: ["class"]` in Tailwind — toggled by adding `.dark` to `<html>`
- Managed by `next-themes` with `attribute="class" defaultTheme="light"`
- **Never use hardcoded hex for colors** — use `text-foreground`, `bg-card`, `text-muted-foreground`, etc.
- **Exception**: Header/subheader glass effects must use explicit `rgba()` values (CSS vars can't be used inside `rgba()` in Tailwind)
- Dark borders use `border-white/[0.08]` (not `border-border`) for hairline glass feel

---

## URL Structure & Breadcrumb Hierarchy

URLs map 1:1 to breadcrumb segments — the path IS the breadcrumb.

```
/projects                                   → Projects
/projects/new                               → Projects / new
/projects/[projectId]                       → Projects / [projectId] / Applications
/projects/[projectId]/workers               → Projects / [projectId] / Workers
/projects/[projectId]/functions             → Projects / [projectId] / Functions
/projects/[projectId]/databases             → Projects / [projectId] / Databases
/projects/[projectId]/settings              → Projects / [projectId] / Settings
/projects/[projectId]/applications/[appId]  → Projects / [projectId] / Applications / [appId]
```

**Rules:**
- Workers, Functions, Databases, Settings are **project-scoped** — always under `/projects/[projectId]/`
- There are no global-level resource pages (no `/workers`, `/functions`, etc.)
- Sidebar resource nav items update their `href` to include the active `[projectId]`
- Parent segments are clickable `<Link>`. Last segment is a styled `<span>` (primary color, `bg-primary/10` pill).
- `[projectId]` segment in the breadcrumb links back to `/projects/[projectId]`

---

## File Locations

| What | Where |
|------|-------|
| CSS tokens | `apps/web/src/app/globals.css` |
| Tailwind config | `apps/web/tailwind.config.ts` |
| Root layout + fonts | `apps/web/src/app/layout.tsx` |
| Header | `apps/web/src/components/layout/header.tsx` |
| Sidebar | `apps/web/src/components/layout/sidebar.tsx` |
| AppShell (breadcrumb + layout) | `apps/web/src/components/layout/app-shell.tsx` |
| ThemeToggle | `apps/web/src/components/theme-toggle.tsx` |
| WorkspaceSwitcher | `apps/web/src/components/workspace-switcher.tsx` |
| Projects listing page | `apps/web/src/app/(authenticated)/projects/page.tsx` |
| Project detail page | `apps/web/src/app/(authenticated)/projects/[projectId]/page.tsx` |
| Project sub-pages | `apps/web/src/app/(authenticated)/projects/[projectId]/{workers,functions,databases,settings}/page.tsx` |
| New project wizard | `apps/web/src/app/(authenticated)/projects/new/page.tsx` |
| Shared UI components | `packages/ui/src/components/` |
| Shared UI tokens | `packages/ui/src/styles/globals.css` |
