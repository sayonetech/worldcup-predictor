import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
import type { RefObject } from "react";
import footballImage from "../assets/football.png";

export type GoalSide = "left" | "right";
export type GoalAnimationMode = "normal" | "argentina-advantage";

export interface GoalBallAnimationHandle {
  play: (side: GoalSide) => void;
}

interface GoalBallAnimationProps {
  mode?: GoalAnimationMode;
  leftTeamCode: string;
  rightTeamCode: string;
  containerRef: RefObject<HTMLElement | null>;
  leftSourceRef: RefObject<HTMLElement | null>;
  rightSourceRef: RefObject<HTMLElement | null>;
  leftTargetRef: RefObject<HTMLElement | null>;
  rightTargetRef: RefObject<HTMLElement | null>;
}

interface Point {
  x: number;
  y: number;
}

interface CurveSegment {
  start: Point;
  control: Point;
  end: Point;
}

type PhaseEasing = "linear" | "momentum-decay";

interface MotionPhase {
  segment: CurveSegment;
  duration: number;
  holdAfter?: number;
  easing?: PhaseEasing;
}

interface Shot {
  id: number;
  mode: GoalAnimationMode;
  direction: "left-to-right" | "right-to-left";
  startSide: GoalSide;
  targetSide: GoalSide;
  phases: MotionPhase[];
  duration: number;
  trailLength: number;
  spinDirection: 1 | -1;
  stampPoint?: Point;
}

const DURATION_MS = 720;
const CURVE_SAMPLES = 140;
const BUTTON_GLOW_MS = 420;
const ARGENTINA_CODE = "ARG";

function getCenter(el: HTMLElement, container: HTMLElement) {
  const rect = el.getBoundingClientRect();
  const containerRect = container.getBoundingClientRect();
  return {
    x: rect.left + rect.width / 2 - containerRect.left,
    y: rect.top + rect.height / 2 - containerRect.top,
  };
}

function curvePoint(segment: CurveSegment, t: number) {
  const inverse = 1 - t;
  return {
    x: inverse * inverse * segment.start.x
      + 2 * inverse * t * segment.control.x
      + t * t * segment.end.x,
    y: inverse * inverse * segment.start.y
      + 2 * inverse * t * segment.control.y
      + t * t * segment.end.y,
  };
}

function curveTangent(segment: CurveSegment, t: number) {
  return {
    x: 2 * (1 - t) * (segment.control.x - segment.start.x)
      + 2 * t * (segment.end.x - segment.control.x),
    y: 2 * (1 - t) * (segment.control.y - segment.start.y)
      + 2 * t * (segment.end.y - segment.control.y),
  };
}

function buildArcLengthTable(segment: CurveSegment) {
  const table = [{ t: 0, length: 0 }];
  let previous = curvePoint(segment, 0);
  let totalLength = 0;

  for (let index = 1; index <= CURVE_SAMPLES; index += 1) {
    const t = index / CURVE_SAMPLES;
    const point = curvePoint(segment, t);
    totalLength += Math.hypot(point.x - previous.x, point.y - previous.y);
    table.push({ t, length: totalLength });
    previous = point;
  }

  return { table, totalLength };
}

function distanceProgressToCurveT(
  progress: number,
  table: ReturnType<typeof buildArcLengthTable>["table"],
  totalLength: number,
) {
  const targetLength = progress * totalLength;
  let low = 1;
  let high = table.length - 1;

  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (table[middle].length < targetLength) low = middle + 1;
    else high = middle;
  }

  const current = table[low];
  const previous = table[Math.max(0, low - 1)];
  const segmentLength = current.length - previous.length;
  const segmentProgress = segmentLength === 0
    ? 0
    : (targetLength - previous.length) / segmentLength;

  return previous.t + (current.t - previous.t) * segmentProgress;
}

function easePhase(progress: number, easing: PhaseEasing = "linear") {
  if (easing === "momentum-decay") {
    // A rebound starts with stored impact energy, then continuously loses
    // velocity to drag/friction until it nearly settles at the destination.
    return 1 - (1 - progress) ** 3.6;
  }
  return progress;
}

function AnimatedShot({ shot, onComplete }: { shot: Shot; onComplete: (id: number) => void }) {
  const shotRef = useRef<HTMLSpanElement>(null);
  const ballRef = useRef<HTMLImageElement>(null);
  const forceRef = useRef<HTMLSpanElement>(null);
  const wallRef = useRef<HTMLSpanElement>(null);
  const stampRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const element = shotRef.current;
    const ball = ballRef.current;
    if (!element || !ball) return;

    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) {
      onComplete(shot.id);
      return;
    }

    const phaseTables = shot.phases.map((phase) => buildArcLengthTable(phase.segment));
    const impactTime = shot.mode === "argentina-advantage"
      ? shot.phases[0].duration
      : -1;
    let frameId = 0;
    let startTime: number | null = null;
    element.style.setProperty("--goal-trail-length", `${shot.trailLength}px`);

    const drawFrame = (timestamp: number) => {
      if (startTime === null) startTime = timestamp;

      const elapsed = Math.min(timestamp - startTime, shot.duration);
      const progress = elapsed / shot.duration;
      let phaseIndex = shot.phases.length - 1;
      let phaseProgress = 1;
      let holdingImpact = false;
      let phaseStart = 0;

      for (let index = 0; index < shot.phases.length; index += 1) {
        const phase = shot.phases[index];
        const phaseEnd = phaseStart + phase.duration;
        const holdEnd = phaseEnd + (phase.holdAfter ?? 0);
        if (elapsed <= phaseEnd) {
          phaseIndex = index;
          phaseProgress = (elapsed - phaseStart) / phase.duration;
          break;
        }
        if (elapsed <= holdEnd) {
          phaseIndex = index;
          phaseProgress = 1;
          holdingImpact = index === 0 && shot.mode === "argentina-advantage";
          break;
        }
        phaseStart = holdEnd;
      }

      const phase = shot.phases[phaseIndex];
      const easedPhaseProgress = easePhase(Math.max(0, phaseProgress), phase.easing);
      const phaseTable = phaseTables[phaseIndex];
      const t = distanceProgressToCurveT(
        easedPhaseProgress,
        phaseTable.table,
        phaseTable.totalLength,
      );
      const point = curvePoint(phase.segment, t);
      const tangent = curveTangent(phase.segment, t);
      const angle = Math.atan2(tangent.y, tangent.x) * (180 / Math.PI);
      const fadeProgress = Math.max(0, (progress - 0.92) / 0.08);
      const opacity = 1 - fadeProgress * fadeProgress;
      const trailRamp = Math.min(progress / 0.12, 1);
      const trailFade = progress < 0.9 ? 1 : Math.max(0, (1 - progress) / 0.1);
      const returning = shot.mode === "argentina-advantage" && phaseIndex > 0;
      const returnTrailReduction = returning ? 1 - easedPhaseProgress * 0.72 : 1;
      const impactDistance = Math.abs(elapsed - impactTime);
      const forcePulse = shot.mode === "argentina-advantage"
        ? Math.max(0, 1 - impactDistance / 105)
        : 0;
      const compression = Math.max(forcePulse, holdingImpact ? 1 : 0);
      const squashX = 1 - compression * 0.18;
      const squashY = 1 + compression * 0.16;
      const shake = compression * Math.sin(elapsed * 0.32) * 1.2;

      element.style.transform = `translate3d(${point.x}px, ${point.y}px, 0) rotate(${angle}deg)`;
      element.style.opacity = String(Math.max(0, opacity));
      element.style.setProperty(
        "--goal-trail-scale",
        String(trailRamp * returnTrailReduction),
      );
      element.style.setProperty(
        "--goal-trail-opacity",
        String(trailFade * returnTrailReduction),
      );
      const rotation = shot.mode === "argentina-advantage"
        ? phaseIndex === 0
          ? phaseProgress * 540
          : 540 + easedPhaseProgress * 150
        : progress * 720;
      ball.style.transform = `translate3d(${shake}px, 0, 0) rotate(${shot.spinDirection * rotation}deg) scale(${squashX}, ${squashY})`;
      if (forceRef.current) {
        forceRef.current.style.opacity = String(forcePulse * 0.62);
        forceRef.current.style.transform = `scale(${0.45 + forcePulse * 1.15})`;
      }
      if (wallRef.current) {
        wallRef.current.style.opacity = String(forcePulse * 0.72);
        wallRef.current.style.transform = `translateY(-50%) scaleY(${0.4 + forcePulse * 0.9})`;
      }
      if (stampRef.current && shot.stampPoint && impactTime >= 0) {
        const since = elapsed - impactTime;
        const appear = Math.min(1, Math.max(0, since) / 90);
        const punch = Math.min(1, Math.max(0, since) / 140);
        const easedPunch = 1 - (1 - punch) ** 3;
        const stampScale = since < 0 ? 1.5 : 1.5 - 0.5 * easedPunch;
        const holdEnd = 950;
        const fadeDuration = 540;
        const fade = since <= holdEnd
          ? 1
          : Math.max(0, 1 - (since - holdEnd) / fadeDuration);
        const stampOpacity = since < 0 ? 0 : appear * fade;
        const stampRotate = -12 + easedPunch * 8;
        stampRef.current.style.opacity = String(stampOpacity);
        stampRef.current.style.transform =
          `translate3d(${shot.stampPoint.x}px, ${shot.stampPoint.y}px, 0)`
          + ` translate(-50%, -50%) rotate(${stampRotate}deg) scale(${stampScale})`;
      }

      if (elapsed < shot.duration) frameId = window.requestAnimationFrame(drawFrame);
      else onComplete(shot.id);
    };

    frameId = window.requestAnimationFrame(drawFrame);
    return () => window.cancelAnimationFrame(frameId);
  }, [onComplete, shot]);

  return (
    <>
      <span
        ref={shotRef}
        className={`goal-animation ${shot.direction}`}
        data-direction={shot.direction}
        data-start-side={shot.startSide}
        data-target-side={shot.targetSide}
        data-animation-mode={shot.mode}
        data-end-area={shot.mode === "argentina-advantage" ? "center" : "opponent-flag"}
        data-motion-phases={shot.mode === "argentina-advantage"
          ? "approach impact upward-rebound momentum-decay"
          : "approach"}
        data-impact-side={shot.mode === "argentina-advantage" ? shot.targetSide : undefined}
        data-var-stamp={shot.stampPoint ? "true" : undefined}
        data-bounce-count="0"
      >
        <span className="goal-trail goal-trail--haze" />
        <span className="goal-trail goal-trail--core" />
        <span ref={forceRef} className="goal-force-ripple" />
        <span ref={wallRef} className="goal-force-wall" />
        <span className="goal-ball-glow" />
        <img ref={ballRef} className="goal-ball" src={footballImage} alt="" />
      </span>
      {shot.stampPoint ? (
        <span ref={stampRef} className="goal-var-stamp" aria-hidden="true">
          <span className="goal-var-stamp__top">⚠ VAR</span>
          <span className="goal-var-stamp__main">NO GOAL</span>
        </span>
      ) : null}
    </>
  );
}

export const GoalBallAnimation = forwardRef<GoalBallAnimationHandle, GoalBallAnimationProps>(
  function GoalBallAnimation(
    {
      mode = "normal",
      leftTeamCode,
      rightTeamCode,
      containerRef,
      leftSourceRef,
      rightSourceRef,
      leftTargetRef,
      rightTargetRef,
    },
    ref,
  ) {
    const [shots, setShots] = useState<Shot[]>([]);
    const nextIdRef = useRef(0);
    const glowTimersRef = useRef<Set<number>>(new Set());

    const removeShot = useCallback((id: number) => {
      setShots((current) => current.filter((shot) => shot.id !== id));
    }, []);

    useEffect(() => () => {
      glowTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    }, []);

    useImperativeHandle(ref, () => ({
      play(side) {
        const container = containerRef.current;
        const source = side === "left" ? leftSourceRef.current : rightSourceRef.current;
        const target = side === "left" ? rightTargetRef.current : leftTargetRef.current;
        if (!container || !source || !target) return;

        source.classList.remove("goal-glow");
        void source.offsetWidth;
        source.classList.add("goal-glow");
        const glowTimer = window.setTimeout(() => {
          source.classList.remove("goal-glow");
          glowTimersRef.current.delete(glowTimer);
        }, BUTTON_GLOW_MS);
        glowTimersRef.current.add(glowTimer);

        if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;

        const start = getCenter(source, container);
        const end = getCenter(target, container);
        const clickedTeamCode = side === "left" ? leftTeamCode : rightTeamCode;
        const defendingTeamCode = side === "left" ? rightTeamCode : leftTeamCode;
        const useArgentinaAdvantage = mode === "argentina-advantage"
          && clickedTeamCode !== ARGENTINA_CODE
          && defendingTeamCode === ARGENTINA_CODE;
        const horizontalDistance = Math.abs(end.x - start.x);
        const directDistance = Math.hypot(end.x - start.x, end.y - start.y);
        const desiredLift = Math.min(118, Math.max(54, horizontalDistance * 0.24));
        const normalSegment: CurveSegment = {
          start,
          control: {
            x: start.x + (end.x - start.x) * 0.44,
            y: Math.max(16, Math.min(start.y, end.y) - desiredLift),
          },
          end,
        };
        let stampPoint: Point | undefined;
        let phases: MotionPhase[] = [{
          segment: normalSegment,
          duration: DURATION_MS,
        }];

        if (useArgentinaAdvantage) {
          const towardCenter = end.x < start.x ? 1 : -1;
          const nearTargetOffset = Math.min(32, Math.max(18, horizontalDistance * 0.065));
          const impact = {
            x: end.x + towardCenter * nearTargetOffset,
            y: end.y + Math.min(8, Math.abs(start.y - end.y) * 0.08),
          };
          stampPoint = impact;
          const approach: CurveSegment = {
            start,
            control: {
              x: start.x + (impact.x - start.x) * 0.5,
              y: Math.max(14, Math.min(start.y, impact.y) - desiredLift),
            },
            end: impact,
          };
          const containerRect = container.getBoundingClientRect();
          const centerPoint = {
            x: containerRect.width / 2,
            y: Math.min(
              containerRect.height - 24,
              Math.max(end.y + 30, (start.y + end.y) / 2 + 18),
            ),
          };
          const softReturn: CurveSegment = {
            start: impact,
            control: {
              x: impact.x + towardCenter * Math.min(82, horizontalDistance * 0.2),
              y: Math.max(
                8,
                impact.y - Math.min(132, Math.max(76, containerRect.height * 0.34)),
              ),
            },
            end: centerPoint,
          };
          phases = [
            { segment: approach, duration: 575, holdAfter: 64 },
            { segment: softReturn, duration: 1960, easing: "momentum-decay" },
          ];
        }

        setShots((current) => [...current, {
          id: ++nextIdRef.current,
          mode: useArgentinaAdvantage ? "argentina-advantage" : "normal",
          direction: side === "left" ? "left-to-right" : "right-to-left",
          startSide: side,
          targetSide: side === "left" ? "right" : "left",
          phases,
          duration: phases.reduce(
            (total, phase) => total + phase.duration + (phase.holdAfter ?? 0),
            0,
          ),
          trailLength: Math.min(58, Math.max(38, directDistance * 0.13)),
          spinDirection: side === "left" ? 1 : -1,
          stampPoint,
        }]);
      },
    }), [
      containerRef,
      leftSourceRef,
      leftTargetRef,
      leftTeamCode,
      mode,
      rightSourceRef,
      rightTargetRef,
      rightTeamCode,
    ]);

    return (
      <div className="goal-animation-layer" aria-hidden="true">
        {shots.map((shot) => (
          <AnimatedShot key={shot.id} shot={shot} onComplete={removeShot} />
        ))}
      </div>
    );
  },
);
