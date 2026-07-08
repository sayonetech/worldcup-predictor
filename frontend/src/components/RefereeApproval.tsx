import { useEffect, useRef, useState } from "react";

const APPROVAL_MS = 1900;

// Prefer a user-supplied cut-out; fall back to the committed cartoon placeholder.
const PRIMARY_SRC = "/referee.png";
const FALLBACK_SRC = "/referee.svg";

/**
 * Visual-only Easter egg: a referee slides in and gives a thumbs-up.
 * Self-timing — calls onDone after the animation so the parent can unmount it.
 * Renders nothing under reduced motion. Uses /referee.png if present, otherwise
 * the committed /referee.svg; hides only if both are missing.
 */
export function RefereeApproval({ onDone }: { onDone: () => void }) {
  const onDoneRef = useRef(onDone);
  useEffect(() => {
    onDoneRef.current = onDone;
  });

  const [src, setSrc] = useState(PRIMARY_SRC);

  const reduceMotion =
    typeof window !== "undefined" &&
    !!window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;

  useEffect(() => {
    if (reduceMotion) {
      onDoneRef.current();
      return;
    }
    const timer = window.setTimeout(() => onDoneRef.current(), APPROVAL_MS);
    return () => window.clearTimeout(timer);
  }, [reduceMotion]);

  if (reduceMotion) return null;

  return (
    <div className="referee-approval" aria-hidden="true">
      <img
        className="referee-approval__img"
        src={src}
        alt=""
        onError={() =>
          src === PRIMARY_SRC ? setSrc(FALLBACK_SRC) : onDoneRef.current()
        }
      />
    </div>
  );
}
