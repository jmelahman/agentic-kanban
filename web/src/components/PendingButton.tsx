import { ButtonHTMLAttributes, ReactNode } from "react";

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> & {
  pending: boolean;
  idleLabel: ReactNode;
  pendingLabel: ReactNode;
};

export function PendingButton({ pending, idleLabel, pendingLabel, disabled, ...rest }: Props) {
  return (
    <button {...rest} disabled={pending || disabled}>
      {pending ? (
        <span className="inline-flex items-center gap-1.5">
          <Spinner />
          {pendingLabel}
        </span>
      ) : (
        idleLabel
      )}
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
