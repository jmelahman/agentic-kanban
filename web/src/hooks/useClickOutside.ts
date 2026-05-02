import { RefObject, useEffect } from "react";

type Options = {
  enabled?: boolean;
  // CSS selectors for elements that should NOT count as "outside" — clicks
  // on these are ignored. Useful for keeping the host open when the user
  // interacts with floating UI rendered as siblings (toasts, popovers).
  exemptSelectors?: string[];
};

export function useClickOutside(
  ref: RefObject<HTMLElement | null>,
  onOutside: (e: MouseEvent) => void,
  { enabled = true, exemptSelectors = [] }: Options = {},
): void {
  useEffect(() => {
    if (!enabled) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Element | null;
      if (!target) return;
      if (ref.current?.contains(target as Node)) return;
      for (const sel of exemptSelectors) {
        if (target.closest(sel)) return;
      }
      onOutside(e);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
    // exemptSelectors is expected to be stable across renders; if callers
    // need dynamic selectors they can recreate the array.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, ref, onOutside, exemptSelectors.join("|")]);
}
