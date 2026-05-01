import { ButtonHTMLAttributes, ReactNode } from "react";

type Variant = "primary" | "secondary" | "neutral" | "danger" | "ghost" | "dashed";
type Size = "sm" | "md" | "lg" | "icon";

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> & {
  variant?: Variant;
  size?: Size;
  pending?: boolean;
  idleLabel?: ReactNode;
  pendingLabel?: ReactNode;
  children?: ReactNode;
};

const BASE = "rounded transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed";

const VARIANTS: Record<Variant, string> = {
  primary: "bg-red-700 text-white hover:bg-red-600",
  secondary: "bg-zinc-700 text-white hover:bg-zinc-600",
  neutral: "bg-zinc-800 text-zinc-300 hover:bg-zinc-700",
  danger: "bg-red-900/60 text-red-100 hover:bg-red-800",
  ghost: "text-zinc-400 hover:text-zinc-100",
  dashed: "border border-dashed border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:border-zinc-600",
};

const SIZES: Record<Size, string> = {
  sm: "px-2 py-0.5 text-xs",
  md: "px-2 py-1",
  lg: "px-3 py-1",
  icon: "p-1",
};

export function Button({
  variant = "neutral",
  size = "md",
  pending = false,
  idleLabel,
  pendingLabel,
  disabled,
  className = "",
  children,
  ...rest
}: Props) {
  const content =
    pending && pendingLabel ? (
      <span className="inline-flex items-center gap-1.5">
        <Spinner />
        {pendingLabel}
      </span>
    ) : (
      idleLabel ?? children
    );
  return (
    <button
      {...rest}
      disabled={pending || disabled}
      className={`${BASE} ${VARIANTS[variant]} ${SIZES[size]} ${className}`}
    >
      {content}
    </button>
  );
}

export function Spinner({ className = "" }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="loading"
      className={`inline-block h-3 w-3 animate-spin rounded-full border border-current border-r-transparent align-[-1px] ${className}`}
    />
  );
}
