import { useRef, useEffect, useCallback, useState } from 'react';

const SECTION_COLORS = [
  '#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4',
  '#3b82f6', '#8b5cf6', '#ec4899', '#14b8a6', '#f59e0b',
  '#a855f7', '#10b981', '#6366f1', '#f43f5e', '#84cc16',
  '#0ea5e9', '#d946ef', '#fb923c', '#7c3aed', '#e11d48',
];

const SECTION_COLORS_DARK = [
  '#dc2626', '#ea580c', '#ca8a04', '#16a34a', '#0891b2',
  '#2563eb', '#7c3aed', '#db2777', '#0d9488', '#d97706',
  '#9333ea', '#059669', '#4f46e5', '#e11d48', '#65a30d',
  '#0284c7', '#c026d3', '#ea580c', '#6d28d9', '#be123c',
];

interface WheelSection {
  id: number;
  username: string;
  colorIndex: number;
  targetArc: number;
  currentArc: number;
  opacity: number;
  destroying: boolean;
}

interface RouletteWheelProps {
  players: { id: number; username: string }[];
  selectedIndex: number;
  isSpinning: boolean;
  spinDurationMs: number;
  eliminatedIds: number[];
  onSpinComplete?: () => void;
  onDestroyComplete?: (playerId: number) => void;
}

function easeOutQuart(t: number): number {
  if (t < 0.08) {
    return 0.08 * (t / 0.08) * (t / 0.08);
  }
  const p = (t - 0.08) / 0.92;
  return 0.08 + 0.92 * (1 - Math.pow(1 - p, 3.5));
}

function calcLandingAngle(selectedIndex: number, playerCount: number): number {
  if (playerCount <= 0) return 0;
  const sliceAngle = 360 / playerCount;
  return ((360 - (selectedIndex + 0.5) * sliceAngle) % 360 + 360) % 360;
}

export function RouletteWheel({
  players,
  selectedIndex,
  isSpinning,
  spinDurationMs,
  eliminatedIds,
  onSpinComplete,
  onDestroyComplete,
}: RouletteWheelProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const rotationRef = useRef(0);
  const sectionsRef = useRef<WheelSection[]>([]);
  const animFrameRef = useRef<number>(0);
  const spinRef = useRef<{ startAngle: number; targetAngle: number; startTime: number; duration: number } | null>(null);
  const prevPlayersRef = useRef<typeof players>([]);
  const colorMapRef = useRef<Map<number, number>>(new Map());
  const pendingDestroyRef = useRef<number | null>(null);
  const [canvasSize, setCanvasSize] = useState(400);
  const isSpinningRef = useRef(false);
  const onSpinCompleteRef = useRef(onSpinComplete);
  const onDestroyCompleteRef = useRef(onDestroyComplete);
  const spinDurationMsRef = useRef(spinDurationMs);
  const playersRef = useRef(players);
  const selectedIndexRef = useRef(selectedIndex);

  onSpinCompleteRef.current = onSpinComplete;
  onDestroyCompleteRef.current = onDestroyComplete;
  spinDurationMsRef.current = spinDurationMs;
  playersRef.current = players;
  selectedIndexRef.current = selectedIndex;

  useEffect(() => {
    const newMap = new Map(colorMapRef.current);
    players.forEach((p, i) => {
      if (!newMap.has(p.id)) newMap.set(p.id, i);
    });
    colorMapRef.current = newMap;
  }, [players]);

  const rebuildSections = useCallback(() => {
    const n = players.length || 1;
    const baseArc = (2 * Math.PI) / n;
    const prevMap = new Map(sectionsRef.current.map((s) => [s.id, s]));

    const newSections: WheelSection[] = players.map((p) => {
      const existing = prevMap.get(p.id);
      const ci = colorMapRef.current.get(p.id) ?? players.indexOf(p);
      return {
        id: p.id,
        username: p.username,
        colorIndex: ci,
        targetArc: baseArc,
        currentArc: existing?.currentArc ?? baseArc,
        opacity: existing?.opacity ?? 1,
        destroying: false,
      };
    });

    const destroyingPlayerId = pendingDestroyRef.current;
    if (destroyingPlayerId !== null) {
      const prev = prevMap.get(destroyingPlayerId);
      if (prev && !newSections.find((s) => s.id === destroyingPlayerId)) {
        newSections.push({
          id: prev.id,
          username: prev.username,
          colorIndex: prev.colorIndex,
          targetArc: 0,
          currentArc: prev.currentArc,
          opacity: prev.opacity,
          destroying: true,
        });
      }
    }

    sectionsRef.current = newSections;
  }, [players]);

  useEffect(() => {
    const currIds = new Set(players.map((p) => p.id));
    const removed = prevPlayersRef.current.filter((p) => !currIds.has(p.id));
    if (removed.length > 0 && pendingDestroyRef.current === null) {
      pendingDestroyRef.current = removed[0].id;
    }
    rebuildSections();
    prevPlayersRef.current = players;
  }, [players, eliminatedIds, rebuildSections]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const size = canvasSize;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = size * dpr;
    canvas.height = size * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    const cx = size / 2;
    const cy = size / 2;
    const outerRadius = size / 2 - 24;
    const hubRadius = outerRadius * 0.17;
    const nameRadius = outerRadius * 0.62;

    ctx.clearRect(0, 0, size, size);

    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate((rotationRef.current * Math.PI) / 180);

    const sections = sectionsRef.current.filter((s) => !s.destroying);
    const destroying = sectionsRef.current.filter((s) => s.destroying);
    const activeSections = [...sections, ...destroying];
    const currentSelectedIndex = selectedIndexRef.current;

    ctx.shadowColor = 'rgba(0,0,0,0.5)';
    ctx.shadowBlur = 25;
    ctx.shadowOffsetY = 6;
    ctx.beginPath();
    ctx.arc(0, 0, outerRadius, 0, Math.PI * 2);
    ctx.fillStyle = '#1a1a2e';
    ctx.fill();
    ctx.shadowColor = 'transparent';
    ctx.shadowBlur = 0;
    ctx.shadowOffsetY = 0;

    let angleOffset = 0;
    for (const section of activeSections) {
      const arc = section.currentArc;
      if (arc < 0.001 && section.destroying) continue;

      const startAngle = angleOffset - Math.PI / 2;
      const endAngle = startAngle + arc;

      ctx.beginPath();
      ctx.moveTo(0, 0);
      ctx.arc(0, 0, outerRadius, startAngle, endAngle);
      ctx.closePath();

      ctx.globalAlpha = section.opacity;

      const baseColor = SECTION_COLORS[section.colorIndex % SECTION_COLORS.length];
      const darkColor = SECTION_COLORS_DARK[section.colorIndex % SECTION_COLORS_DARK.length];

      const gradient = ctx.createRadialGradient(0, 0, hubRadius * 0.8, 0, 0, outerRadius);
      gradient.addColorStop(0, darkColor);
      gradient.addColorStop(0.35, baseColor);
      gradient.addColorStop(1, baseColor);
      ctx.fillStyle = gradient;
      ctx.fill();

      ctx.globalAlpha = 1;
      ctx.strokeStyle = 'rgba(0,0,0,0.5)';
      ctx.lineWidth = 2.5;
      ctx.stroke();

      const sectionIndex = sections.indexOf(section);
      if (sectionIndex === currentSelectedIndex && !section.destroying && currentSelectedIndex >= 0 && currentSelectedIndex < sections.length) {
        ctx.save();
        ctx.globalAlpha = 0.25 * section.opacity;
        ctx.beginPath();
        ctx.moveTo(0, 0);
        ctx.arc(0, 0, outerRadius, startAngle, endAngle);
        ctx.closePath();
        ctx.fillStyle = '#fff';
        ctx.fill();
        ctx.restore();
      }

      ctx.globalAlpha = 1;

      if (section.currentArc > 0.12) {
        const midAngle = startAngle + arc / 2;
        ctx.save();
        ctx.rotate(midAngle);
        const maxFontSize = Math.min(15, (arc * outerRadius) / 4);
        const fontSize = Math.max(9, maxFontSize);
        ctx.font = `bold ${fontSize}px 'Oxanium Variable', sans-serif`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = section.destroying ? 'rgba(255,255,255,0.3)' : 'white';
        ctx.shadowColor = 'rgba(0,0,0,0.8)';
        ctx.shadowBlur = 3;

        const maxChars = Math.max(2, Math.floor((arc * nameRadius) / (fontSize * 0.55)));
        let displayName = section.username;
        if (displayName.length > maxChars) {
          displayName = displayName.slice(0, maxChars - 1) + '…';
        }

        ctx.fillText(displayName, nameRadius, 0);
        ctx.shadowBlur = 0;
        ctx.restore();
      }

      angleOffset += arc;
    }

    ctx.globalAlpha = 1;

    const outerRingWidth = outerRadius * 0.035;
    ctx.beginPath();
    ctx.arc(0, 0, outerRadius, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(220,220,220,0.7)';
    ctx.lineWidth = outerRingWidth;
    ctx.stroke();

    ctx.beginPath();
    ctx.arc(0, 0, outerRadius + outerRingWidth / 2, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(180,180,180,0.3)';
    ctx.lineWidth = 1;
    ctx.stroke();

    const sectionArcs = sections.map((s) => s.currentArc);
    let divAngle = -Math.PI / 2;
    for (const arc of sectionArcs) {
      ctx.beginPath();
      ctx.moveTo(Math.cos(divAngle) * hubRadius, Math.sin(divAngle) * hubRadius);
      ctx.lineTo(Math.cos(divAngle) * outerRadius, Math.sin(divAngle) * outerRadius);
      ctx.strokeStyle = 'rgba(0,0,0,0.4)';
      ctx.lineWidth = 1.5;
      ctx.stroke();
      divAngle += arc;
    }

    const hubGradient = ctx.createRadialGradient(
      -hubRadius * 0.3, -hubRadius * 0.3, 0,
      0, 0, hubRadius
    );
    hubGradient.addColorStop(0, '#666');
    hubGradient.addColorStop(0.5, '#333');
    hubGradient.addColorStop(1, '#1a1a2e');
    ctx.beginPath();
    ctx.arc(0, 0, hubRadius, 0, Math.PI * 2);
    ctx.fillStyle = hubGradient;
    ctx.fill();
    ctx.strokeStyle = 'rgba(255,255,255,0.35)';
    ctx.lineWidth = 3;
    ctx.stroke();

    ctx.beginPath();
    ctx.arc(0, 0, hubRadius * 0.55, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(255,255,255,0.15)';
    ctx.lineWidth = 1.5;
    ctx.stroke();

    ctx.restore();

    const pointerSize = size * 0.055;
    const pointerTipY = cy - outerRadius + 2;
    const pointerBaseY = pointerTipY - pointerSize * 2.2;

    ctx.save();
    ctx.shadowColor = 'rgba(0,0,0,0.5)';
    ctx.shadowBlur = 8;
    ctx.shadowOffsetY = 2;
    ctx.beginPath();
    ctx.moveTo(cx, pointerTipY);
    ctx.lineTo(cx - pointerSize * 0.7, pointerBaseY);
    ctx.lineTo(cx + pointerSize * 0.7, pointerBaseY);
    ctx.closePath();
    const pointerGrad = ctx.createLinearGradient(cx, pointerBaseY, cx, pointerTipY);
    pointerGrad.addColorStop(0, '#fbbf24');
    pointerGrad.addColorStop(0.4, '#f59e0b');
    pointerGrad.addColorStop(1, '#d97706');
    ctx.fillStyle = pointerGrad;
    ctx.fill();
    ctx.strokeStyle = 'rgba(255,255,255,0.6)';
    ctx.lineWidth = 1.5;
    ctx.stroke();
    ctx.shadowColor = 'transparent';
    ctx.shadowBlur = 0;
    ctx.shadowOffsetY = 0;
    ctx.restore();

    ctx.beginPath();
    ctx.arc(cx, pointerBaseY + pointerSize * 0.4, pointerSize * 0.18, 0, Math.PI * 2);
    ctx.fillStyle = 'rgba(255,255,255,0.7)';
    ctx.fill();
  }, [canvasSize]);

  const animateSections = useCallback(() => {
    const sections = sectionsRef.current;
    let changed = false;
    const activeCount = sections.filter((s) => !s.destroying).length || 1;
    for (const section of sections) {
      if (section.destroying) {
        section.currentArc += (section.targetArc - section.currentArc) * 0.12;
        section.opacity += (0 - section.opacity) * 0.12;
        if (section.currentArc < 0.005 && section.opacity < 0.02) {
          section.currentArc = 0;
          section.opacity = 0;
          if (pendingDestroyRef.current === section.id) {
            pendingDestroyRef.current = null;
            onDestroyCompleteRef.current?.(section.id);
          }
        }
        changed = true;
      } else if (Math.abs(section.currentArc - section.targetArc) > 0.001) {
        section.targetArc = (2 * Math.PI) / activeCount;
        section.currentArc += (section.targetArc - section.currentArc) * 0.12;
        if (Math.abs(section.currentArc - section.targetArc) < 0.001) {
          section.currentArc = section.targetArc;
        }
        changed = true;
      }
    }
    sectionsRef.current = sections.filter((s) => !(s.destroying && s.currentArc < 0.005 && s.opacity < 0.02));
    return changed;
  }, []);

  useEffect(() => {
    let active = true;

    const loop = () => {
      if (!active) return;
      let needsRedraw = false;

      if (spinRef.current) {
        const { startAngle, targetAngle, startTime, duration } = spinRef.current;
        const elapsed = performance.now() - startTime;
        const progress = Math.min(elapsed / duration, 1);
        const easedProgress = easeOutQuart(progress);
        rotationRef.current = startAngle + (targetAngle - startAngle) * easedProgress;

        if (progress >= 1) {
          rotationRef.current = targetAngle;
          spinRef.current = null;
          isSpinningRef.current = false;
          onSpinCompleteRef.current?.();
        }
        needsRedraw = true;
      }

      if (animateSections()) {
        needsRedraw = true;
      }

      if (needsRedraw) {
        draw();
      }

      animFrameRef.current = requestAnimationFrame(loop);
    };

    draw();
    animFrameRef.current = requestAnimationFrame(loop);
    return () => {
      active = false;
      if (animFrameRef.current) cancelAnimationFrame(animFrameRef.current);
    };
  }, [draw, animateSections]);

  useEffect(() => {
    if (isSpinning && !spinRef.current) {
      const playerCount = playersRef.current.length;
      const idx = selectedIndexRef.current;
      if (playerCount <= 0 || idx < 0 || idx >= playerCount) return;

      const currentRotation = rotationRef.current;
      const landingAngle = calcLandingAngle(idx, playerCount);
      const currentEffective = ((currentRotation % 360) + 360) % 360;
      const delta = ((landingAngle - currentEffective) % 360 + 360) % 360;
      const fullSpins = 4 + Math.floor(Math.random() * 3);
      const targetAngle = currentRotation + delta + fullSpins * 360;

      spinRef.current = {
        startAngle: currentRotation,
        targetAngle,
        startTime: performance.now(),
        duration: spinDurationMsRef.current || 5500,
      };
      isSpinningRef.current = true;
    }
  }, [isSpinning]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        const size = Math.min(width, height, 480);
        if (size > 0) setCanvasSize(Math.round(size));
      }
    });

    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    rebuildSections();
  }, [players, rebuildSections]);

  return (
    <div ref={containerRef} className="relative flex w-full items-center justify-center">
      <canvas
        ref={canvasRef}
        style={{ width: canvasSize, height: canvasSize }}
        className="max-w-full"
      />
    </div>
  );
}