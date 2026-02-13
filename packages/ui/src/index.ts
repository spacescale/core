// Utility
export { cn } from "./lib/utils";

// Core components (ported from apps/web)
export { Button, buttonVariants, type ButtonProps } from "./components/button";
export { Badge, badgeVariants, type BadgeProps } from "./components/badge";
export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardDescription,
  CardContent,
} from "./components/card";
export { Input, type InputProps } from "./components/input";
export { Label } from "./components/label";
export { Textarea, type TextareaProps } from "./components/textarea";
export { Skeleton } from "./components/skeleton";
export { Tabs, TabsList, TabsTrigger, TabsContent } from "./components/tabs";
export {
  Dialog,
  DialogPortal,
  DialogOverlay,
  DialogClose,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "./components/dialog";
export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuGroup,
  DropdownMenuPortal,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuRadioGroup,
} from "./components/dropdown-menu";
export {
  Select,
  SelectGroup,
  SelectValue,
  SelectTrigger,
  SelectContent,
  SelectLabel,
  SelectItem,
  SelectSeparator,
  SelectScrollUpButton,
  SelectScrollDownButton,
} from "./components/select";
export {
  type ToastProps,
  type ToastActionElement,
  ToastProvider,
  ToastViewport,
  Toast,
  ToastTitle,
  ToastDescription,
  ToastClose,
  ToastAction,
} from "./components/toast";
export {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "./components/tooltip";

// New components (extracted from SpaceScale designs)
export {
  GlassCard,
  GlassCardHeader,
  GlassCardContent,
  GlassCardFooter,
  glassCardVariants,
  type GlassCardProps,
} from "./components/glass-card";
export {
  StatusIndicator,
  statusIndicatorVariants,
  type StatusIndicatorProps,
} from "./components/status-indicator";
export { SearchInput, type SearchInputProps } from "./components/search-input";
export { Avatar, AvatarImage, AvatarFallback } from "./components/avatar";
export {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "./components/breadcrumb";
export { Switch, type SwitchProps } from "./components/switch";
export {
  MetricCard,
  metricCardVariants,
  type MetricCardProps,
} from "./components/metric-card";
export {
  ProgressBar,
  progressBarVariants,
  type ProgressBarProps,
} from "./components/progress-bar";
export {
  StepIndicator,
  type StepIndicatorProps,
  type Step,
} from "./components/step-indicator";
export { CodeBlock, type CodeBlockProps } from "./components/code-block";
export { EmptyState, type EmptyStateProps } from "./components/empty-state";
export { ViewToggle, type ViewToggleProps } from "./components/view-toggle";
export { Separator } from "./components/separator";
export {
  LogEntry,
  type LogEntryProps,
  type LogLevel,
} from "./components/log-entry";
export { Sparkline, type SparklineProps } from "./components/sparkline";
export { LogoMark, type LogoMarkProps } from "./components/logo-mark";
