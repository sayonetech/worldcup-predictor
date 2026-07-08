import { useEffect, useRef } from "react";

const APPROVAL_MS = 1900;

/**
 * Visual-only Easter egg: a referee slides in and gives a thumbs-up.
 * Self-timing — calls onDone after the animation so the parent can unmount it.
 * Renders nothing under reduced motion; hides itself if /referee.png is missing.
 */
export function RefereeApproval({ onDone }: { onDone: () => void }) {
  const onDoneRef = useRef(onDone);
  useEffect(() => {
    onDoneRef.current = onDone;
  });

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
        src="/referee.png"
        alt=""
        onError={() => onDoneRef.current()}
      />
    </div>
  );
}
