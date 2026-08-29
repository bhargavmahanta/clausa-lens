# Orbital Command Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the placeholder five-card overview with a Figma-faithful, backend-driven four-card command center whose cards move along one smooth tilted 3D orbit.

**Architecture:** Pure orbit helpers map a shared angular offset to continuous CSS 3D transforms; a client-side `OverviewCarousel` uses GSAP to tween only that shared offset and reapplies every card transform on each frame. `CommandCenter` remains the sole owner of decoded backend truth, while a separate ambient scene owns Figma imagery, parallax, and glow animation.

**Tech Stack:** Next.js 16.2.11, React 19.2, TypeScript 6, project CSS tokens, GSAP 3, Vitest 4, Figma-exported PNG/SVG assets.

**Spec:** `docs/superpowers/specs/2026-08-30-orbital-command-center-design.md`

## Global Constraints

- The frontend displays only decoded Core API evidence; it never invents incident, replay, isolation, oracle, or diff results.
- Keep existing incident selection, golden-demo trigger, capsule compilation, baseline replay, what-if replay, diff creation, error handling, and reset confirmation behavior intact.
- Use exactly four overview objects: Incident / Trace, Replay Capsule, Replay, and Diff.
- Keep dashboard cards as accessible React DOM; do not add Three.js, React Three Fiber, Zustand, TanStack Query, or Tailwind.
- Download and commit exact Figma assets; do not rely on expiring Figma URLs or hand-drawn asset approximations.
- Preserve keyboard navigation, 44 px targets, reduced motion, and a 320 px minimum viewport.

---

### Task 1: Continuous orbit geometry and GSAP dependency

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Replace: `web/src/features/overview/carousel.ts`
- Replace: `web/tests/overview/carousel.test.ts`

**Interfaces:**
- Consumes: four stage indices and one shared orbit angle in radians.
- Produces: `overviewCarouselStages`, `OverviewCarouselStage`, `OrbitGeometry`, `OrbitTransform`, `desktopOrbitGeometry`, `normalizeOrbitAngle(angle)`, `getShortestOrbitDelta(currentAngle, targetAngle)`, `getStageTargetAngle(stageIndex, count)`, `getFocusedStageIndex(orbitAngle, count)`, and `getOrbitTransform(stageIndex, orbitAngle, count, geometry)`.

- [ ] **Step 1: Replace the discrete-placement tests with failing continuous-geometry tests**

```ts
import { describe, expect, it } from "vitest";

describe("overview carousel orbit", () => {
  it("defines the four evidence objects in workflow order", async () => {
    const { overviewCarouselStages } = await import("../../src/features/overview/carousel");
    expect(overviewCarouselStages).toEqual(["incident", "capsule", "replay", "diff"]);
  });

  it("places the selected stage at the front and its opposite behind", async () => {
    const { getOrbitTransform, getStageTargetAngle } = await import("../../src/features/overview/carousel");
    const angle = getStageTargetAngle(2, 4);
    const selected = getOrbitTransform(2, angle, 4);
    const opposite = getOrbitTransform(0, angle, 4);
    expect(selected.frontness).toBeCloseTo(1, 5);
    expect(selected.x).toBeCloseTo(0, 5);
    expect(selected.z).toBeGreaterThan(opposite.z);
    expect(selected.scale).toBeGreaterThan(opposite.scale);
    expect(selected.blur).toBeLessThan(opposite.blur);
  });

  it("uses the shortest curved rotation across the wrap boundary", async () => {
    const { getShortestOrbitDelta } = await import("../../src/features/overview/carousel");
    expect(getShortestOrbitDelta(Math.PI * 0.95, -Math.PI * 0.95)).toBeCloseTo(Math.PI * 0.1, 5);
    expect(getShortestOrbitDelta(-Math.PI * 0.95, Math.PI * 0.95)).toBeCloseTo(-Math.PI * 0.1, 5);
  });

  it("changes position continuously between two orbit samples", async () => {
    const { getOrbitTransform } = await import("../../src/features/overview/carousel");
    const start = getOrbitTransform(1, 0, 4);
    const middle = getOrbitTransform(1, Math.PI / 8, 4);
    const end = getOrbitTransform(1, Math.PI / 4, 4);
    expect(middle.x).not.toBe(start.x);
    expect(middle.x).not.toBe(end.x);
    expect(middle.z).toBeGreaterThan(Math.min(start.z, end.z));
    expect(middle.z).toBeLessThan(Math.max(start.z, end.z));
  });
});
```

- [ ] **Step 2: Run the orbit test and verify the old five-slot implementation fails**

Run: `npm test -- tests/overview/carousel.test.ts`

Expected: FAIL because the stage names and continuous-orbit exports do not exist.

- [ ] **Step 3: Install GSAP using the project package manager**

Run: `npm install gsap@^3.13.0`

Expected: `web/package.json` and `web/package-lock.json` record GSAP without changing the pinned Next.js or React versions.

- [ ] **Step 4: Implement the pure orbit model**

```ts
export const overviewCarouselStages = ["incident", "capsule", "replay", "diff"] as const;
export type OverviewCarouselStage = (typeof overviewCarouselStages)[number];

export type OrbitGeometry = {
  radiusX: number;
  radiusY: number;
  depth: number;
  tilt: number;
  minScale: number;
  maxBlur: number;
};

export type OrbitTransform = {
  x: number;
  y: number;
  z: number;
  scale: number;
  rotateX: number;
  rotateY: number;
  rotateZ: number;
  opacity: number;
  blur: number;
  zIndex: number;
  frontness: number;
};

export const desktopOrbitGeometry: OrbitGeometry = {
  radiusX: 520,
  radiusY: 170,
  depth: 520,
  tilt: -0.14,
  minScale: 0.56,
  maxBlur: 4,
};

export function normalizeOrbitAngle(angle: number): number {
  const turn = Math.PI * 2;
  return ((angle + Math.PI) % turn + turn) % turn - Math.PI;
}

export function getShortestOrbitDelta(currentAngle: number, targetAngle: number): number {
  return normalizeOrbitAngle(targetAngle - currentAngle);
}

export function getStageTargetAngle(stageIndex: number, count = overviewCarouselStages.length): number {
  return normalizeOrbitAngle(-(stageIndex * Math.PI * 2) / count);
}
```

```ts
export function getFocusedStageIndex(orbitAngle: number, count = overviewCarouselStages.length): number {
  let focused = 0;
  let closest = Number.POSITIVE_INFINITY;
  for (let index = 0; index < count; index += 1) {
    const distance = Math.abs(normalizeOrbitAngle(orbitAngle + (index * Math.PI * 2) / count));
    if (distance < closest) {
      focused = index;
      closest = distance;
    }
  }
  return focused;
}

export function getOrbitTransform(
  stageIndex: number,
  orbitAngle: number,
  count = overviewCarouselStages.length,
  geometry = desktopOrbitGeometry,
): OrbitTransform {
  const theta = normalizeOrbitAngle(orbitAngle + (stageIndex * Math.PI * 2) / count);
  const rawX = Math.sin(theta) * geometry.radiusX;
  const rawY = (1 - Math.cos(theta)) * geometry.radiusY;
  const tiltCos = Math.cos(geometry.tilt);
  const tiltSin = Math.sin(geometry.tilt);
  const frontness = (Math.cos(theta) + 1) / 2;
  return {
    x: rawX * tiltCos - rawY * tiltSin,
    y: rawX * tiltSin + rawY * tiltCos,
    z: (Math.cos(theta) - 1) * geometry.depth,
    scale: geometry.minScale + (1 - geometry.minScale) * frontness,
    rotateX: (1 - frontness) * 8,
    rotateY: Math.sin(theta) * -24,
    rotateZ: Math.sin(theta) * 7,
    opacity: 0.34 + frontness * 0.66,
    blur: geometry.maxBlur * (1 - frontness),
    zIndex: Math.round(frontness * 100),
    frontness,
  };
}
```

- [ ] **Step 5: Run the orbit tests and verify they pass**

Run: `npm test -- tests/overview/carousel.test.ts`

Expected: PASS with four orbit tests.

- [ ] **Step 6: Commit the geometry slice**

```bash
git add web/package.json web/package-lock.json web/src/features/overview/carousel.ts web/tests/overview/carousel.test.ts
git commit -m "feat(web): add continuous 3D orbit geometry"
```

---

### Task 2: Accessible orbital card scene

**Files:**
- Create: `web/src/features/overview/overview-card.tsx`
- Replace: `web/src/features/overview/overview-carousel.tsx`
- Modify: `web/src/features/overview/index.ts`
- Replace: `web/tests/components/overview-carousel.test.tsx`

**Interfaces:**
- Consumes: Task 1 orbit helpers and `CarouselStageContent[]` from `OverviewCarousel` props.
- Produces: `CarouselStageChip`, `CarouselStageMetric`, `CarouselStageContent`, `OverviewCard`, and `OverviewCarousel` with props `{ stages; initialStage?; autoRotateMs? }`.

- [ ] **Step 1: Write failing server-render tests for four cards and accessible states**

```tsx
const stages = [
  { stage: "incident", eyebrow: "Captured evidence", title: "Incident / Trace", description: "Follow the request path", summary: "Gateway → Checkout → Payment → Ledger", href: "#incident-workspace", actionLabel: "Inspect incident" },
  { stage: "capsule", eyebrow: "Replay artifact", title: "Replay Capsule", description: "Integrity and isolation", summary: "Awaiting capsule compilation", href: "#replay-lab", actionLabel: "Open capsule" },
  { stage: "replay", eyebrow: "Controlled execution", title: "Replay", description: "Baseline and what-if", summary: "Baseline not started", href: "#replay-lab", actionLabel: "Open replay lab" },
  { stage: "diff", eyebrow: "Evidence delta", title: "Diff", description: "First meaningful divergence", summary: "Awaiting both runs", href: "#replay-lab", actionLabel: "Inspect diff" },
] as const;

const markup = renderToStaticMarkup(<OverviewCarousel stages={[...stages]} />);
expect(markup.match(/data-stage=/g)).toHaveLength(4);
expect(markup.match(/aria-current="true"/g)).toHaveLength(1);
expect(markup).toContain("Incident / Trace");
expect(markup).toContain("Replay Capsule");
expect(markup).toContain('aria-label="Rotate orbit backward"');
expect(markup).toContain('aria-label="Rotate orbit forward"');
expect(markup).toContain('aria-label="Pause automatic rotation"');
expect(markup).toContain('aria-roledescription="3D card orbit"');
```

Add a second test with `initialStage="replay"` that expects `Replay in focus` in the live region and exactly one `data-front="true"` card.

- [ ] **Step 2: Run the component test and verify the current five-card markup fails**

Run: `npm test -- tests/components/overview-carousel.test.tsx`

Expected: FAIL on card count, stage names, and orbit control labels.

- [ ] **Step 3: Implement `OverviewCard` as a normal DOM card**

```tsx
export type CarouselStageMetric = { label: string; value: string };

export function OverviewCard({ content, isFront, onPromote, articleRef }: OverviewCardProps) {
  return (
    <article
      aria-current={isFront ? "true" : undefined}
      className="orbit-card"
      data-front={isFront ? "true" : "false"}
      data-stage={content.stage}
      ref={articleRef}
    >
      <button
        aria-label={isFront ? undefined : `Bring ${content.title} forward`}
        className="orbit-card__promote"
        disabled={isFront}
        onClick={onPromote}
        type="button"
      />
      <div className="orbit-card__inner" inert={isFront ? undefined : true}>
        <p className="orbit-card__eyebrow">{content.eyebrow}</p>
        <h2>{content.title}</h2>
        <p>{content.description}</p>
        <strong>{content.summary}</strong>
        <a href={content.href}>{content.actionLabel}</a>
      </div>
    </article>
  );
}
```

Render optional status and metric values as real text. The full-card content remains inert when behind; the overlay promote button remains the only interactive element on a background card.

- [ ] **Step 4: Implement the shared-angle GSAP scene**

```tsx
const orbit = useRef({ angle: getStageTargetAngle(initialIndex) });
const animation = useRef<gsap.core.Tween | null>(null);

const applyTransforms = useCallback(() => {
  cardRefs.current.forEach((card, index) => {
    if (!card) return;
    const value = getOrbitTransform(index, orbit.current.angle, stages.length);
    gsap.set(card, {
      x: value.x,
      y: value.y,
      z: value.z,
      scale: value.scale,
      rotateX: value.rotateX,
      rotateY: value.rotateY,
      rotateZ: value.rotateZ,
      opacity: value.opacity,
      filter: `blur(${value.blur}px)`,
      zIndex: value.zIndex,
    });
  });
}, [stages.length]);

const promote = useCallback((stage: OverviewCarouselStage) => {
  const index = stages.findIndex((item) => item.stage === stage);
  const target = getStageTargetAngle(index, stages.length);
  const destination = orbit.current.angle + getShortestOrbitDelta(orbit.current.angle, target);
  animation.current?.kill();
  if (reduceMotion) {
    orbit.current.angle = destination;
    applyTransforms();
    setActiveStage(stage);
    return;
  }
  animation.current = gsap.to(orbit.current, {
    angle: destination,
    duration: 0.9,
    ease: "power3.inOut",
    onUpdate: applyTransforms,
    onComplete: () => {
      orbit.current.angle = normalizeOrbitAngle(destination);
      setActiveStage(stage);
      cardRefs.current[index]?.focus();
    },
  });
}, [applyTransforms, reduceMotion, stages]);
```

Use this same `promote` function from click, previous/next buttons, arrow keys, auto-rotation, and the pointer-up snap decision. Pointer-down stores `clientX`; pointer-up promotes the adjacent stage only when the signed delta exceeds 48 px.

Add pointer drag with capture, a 48 px snap threshold, keyboard arrows, pause/resume, hover/focus pause, and timer rotation. Use `useReducedMotion` only to choose instant focus and disable automatic movement. Preserve a polite live region and request focus on the promoted front article after the transition completes.

- [ ] **Step 5: Run the overview component and orbit tests**

Run: `npm test -- tests/components/overview-carousel.test.tsx tests/overview/carousel.test.ts`

Expected: PASS with four-card markup and continuous geometry coverage.

- [ ] **Step 6: Commit the accessible scene**

```bash
git add web/src/features/overview web/tests/components/overview-carousel.test.tsx
git commit -m "feat(web): build accessible orbital overview scene"
```

---

### Task 3: Bind the four overview objects to workflow truth

**Files:**
- Modify: `web/src/features/command-center/command-center.tsx`
- Modify: `web/tests/components/command-center.test.tsx`
- Modify: `web/tests/components/home-page.test.tsx`

**Interfaces:**
- Consumes: existing incident selection, capsule reducer, baseline run, what-if run, diff, and isolation evidence already owned by `CommandCenter`.
- Produces: four `CarouselStageContent` objects passed to `OverviewCarousel`; no new data store or API calls.

- [ ] **Step 1: Add a failing command-center test for the four truthful objects**

```tsx
const markup = renderToStaticMarkup(<CommandCenter />);
expect(markup).toContain("Incident / Trace");
expect(markup).toContain("Replay Capsule");
expect(markup).toContain("Baseline and what-if");
expect(markup).toContain("First meaningful divergence");
expect(markup).toContain("No incident selected");
expect(markup).toContain("Awaiting capsule compilation");
expect(markup.match(/data-stage=/g)).toHaveLength(4);
expect(markup).not.toContain('data-stage="overview"');
expect(markup).not.toContain('data-stage="capture"');
```

Keep the existing assertions proving `cap-8271`, `REPRODUCED`, and `MITIGATED` are absent before API responses arrive.

- [ ] **Step 2: Run the command-center tests and verify they fail on the current five-card data**

Run: `npm test -- tests/components/command-center.test.tsx tests/components/home-page.test.tsx`

Expected: FAIL because Capsule is not a stage and Capture/Overview still are.

- [ ] **Step 3: Derive the four stage objects from existing reducer state**

```ts
const carouselStages: CarouselStageContent[] = [
  {
    stage: "incident",
    eyebrow: "Captured evidence",
    title: "Incident / Trace",
    description: "Follow the request through Gateway → Checkout → Payment → Ledger",
    summary: selectedIncidentId ?? "No incident selected",
    href: "#incident-workspace",
    actionLabel: "Inspect incident evidence",
  },
  {
    stage: "capsule",
    eyebrow: "Replay artifact",
    title: "Replay Capsule",
    description: "Integrity, fixtures, policy, and isolation readiness",
    summary: state.capsule.status === "ready" ? state.capsule.value.capsule_id : "Awaiting capsule compilation",
    href: "#replay-lab",
    actionLabel: "Open capsule workflow",
  },
  {
    stage: "replay",
    eyebrow: "Controlled execution",
    title: "Replay",
    description: "Baseline and what-if",
    summary: `${baselineSummary} · ${whatIfSummary}`,
    href: "#replay-lab",
    actionLabel: "Open replay lab",
  },
  {
    stage: "diff",
    eyebrow: "Evidence delta",
    title: "Diff",
    description: "First meaningful divergence",
    summary: diffSummary,
    href: "#replay-lab",
    actionLabel: "Inspect replay diff",
  },
];
```

Add only metrics that can be derived from available response fields. Map loading and error states to truthful text; never replace them with optimistic success labels.

- [ ] **Step 4: Run the focused command-center tests**

Run: `npm test -- tests/components/command-center.test.tsx tests/components/home-page.test.tsx tests/components/overview-carousel.test.tsx`

Expected: PASS and no fixture result values appear before API responses.

- [ ] **Step 5: Commit the workflow binding**

```bash
git add web/src/features/command-center/command-center.tsx web/tests/components/command-center.test.tsx web/tests/components/home-page.test.tsx
git commit -m "feat(web): bind orbit cards to workflow evidence"
```

---

### Task 4: Figma ambient scene, glass styling, and responsive shell

**Files:**
- Create: `web/src/components/system/ambient-scene.tsx`
- Modify: `web/src/components/system/index.ts`
- Modify: `web/src/components/system/command-center-shell.tsx`
- Modify: `web/src/app/page.tsx`
- Modify: `web/src/app/layout.tsx`
- Modify: `web/src/app/globals.css`
- Add: `web/public/figma/command-center-room.png`
- Add: exact Figma-exported shell icons under `web/public/figma/icons/`
- Modify: `web/tests/components/foundation.test.tsx`
- Modify: `web/tests/components/home-page.test.tsx`

**Interfaces:**
- Consumes: existing Figma component PNGs plus the exact room background and shell icon exports from nodes `9:5`, `13:36`, and `13:37` in file `cye6REVv4Tix6qM8OkKEr6`.
- Produces: `AmbientScene` with independent pointer parallax and irregular glow; refined shell/header/sidebar markup; responsive CSS for the orbit and workspace.

- [ ] **Step 1: Write failing foundation tests for exact assets and scene semantics**

```tsx
const markup = renderToStaticMarkup(<HomePage />);
expect(markup).toContain('class="ambient-scene"');
expect(markup).toContain('/figma/command-center-room.png');
expect(markup).toContain('class="ambient-ring-glow"');
expect(markup).toContain('aria-roledescription="3D card orbit"');

const stylesheet = readFileSync(resolve(process.cwd(), "src/app/globals.css"), "utf8");
expect(stylesheet).toMatch(/\.orbit-stage\s*\{[^}]*perspective:\s*1600px;/s);
expect(stylesheet).toMatch(/\.orbit-card\s*\{[^}]*transform-style:\s*preserve-3d;/s);
expect(stylesheet).toMatch(/@media \(prefers-reduced-motion: reduce\)/);
expect(stylesheet).toMatch(/min-height:\s*44px/);
```

- [ ] **Step 2: Run foundation tests and verify the new ambient contract fails**

Run: `npm test -- tests/components/foundation.test.tsx tests/components/home-page.test.tsx`

Expected: FAIL because the room asset, glow class, and new orbit selectors are missing.

- [ ] **Step 3: Download the exact Figma assets**

Use the Figma asset output for the dashboard room and exported shell vectors. Save immutable bytes in `web/public/figma/`; verify image dimensions with `file web/public/figma/* web/public/figma/icons/*`. Reuse the existing ring, sphere, plant, monolith, and stone PNGs when their dimensions and subjects match node `13:37`.

- [ ] **Step 4: Implement `AmbientScene` with layered source imagery**

```tsx
export function AmbientScene() {
  const sceneRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const root = sceneRef.current;
    if (!root || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const move = (event: PointerEvent) => {
      root.style.setProperty("--parallax-x", String(event.clientX / innerWidth - 0.5));
      root.style.setProperty("--parallax-y", String(event.clientY / innerHeight - 0.5));
    };
    const glow = root.querySelector(".ambient-ring-glow");
    const pulse = gsap.timeline({ repeat: -1, repeatDelay: 3.4 })
      .to(glow, { opacity: 0.93, duration: 1.8, ease: "sine.inOut" })
      .to(glow, { opacity: 0.72, duration: 0.08 })
      .to(glow, { opacity: 1, duration: 0.12 })
      .to(glow, { opacity: 0.88, duration: 2.1, ease: "sine.inOut" });
    window.addEventListener("pointermove", move, { passive: true });
    return () => {
      pulse.kill();
      window.removeEventListener("pointermove", move);
    };
  }, []);

  return (
    <div className="ambient-scene" aria-hidden="true" ref={sceneRef}>
      <Image alt="" className="ambient-room" fill priority src="/figma/command-center-room.png" />
      <span className="ambient-ring-glow" />
      <Image alt="" className="ambient-object ambient-object--ring" height={1199} src="/figma/ambient-ring.png" width={1312} />
      <Image alt="" className="ambient-object ambient-object--plant" height={1285} src="/figma/ambient-plant.png" width={1224} />
      <Image alt="" className="ambient-object ambient-object--stone" height={1024} src="/figma/ambient-stone.png" width={1536} />
      <Image alt="" className="ambient-object ambient-object--monolith" height={1536} src="/figma/ambient-monolith.png" width={1024} />
      <Image alt="" className="ambient-object ambient-object--orb" height={1254} src="/figma/ambient-orb.png" width={1254} />
    </div>
  );
}
```

- [ ] **Step 5: Convert the Figma measurements into project CSS**

Configure `Manrope` in `layout.tsx` with `subsets: ["latin"]` and `variable: "--font-manrope"`, apply that variable class to `<body>`, retain the project monospace fallback for evidence IDs, and map Figma fills to reusable tokens:

```css
:root {
  --canvas: #060606;
  --glass-bg: rgba(22, 20, 18, 0.58);
  --glass-inner: rgba(255, 255, 255, 0.035);
  --glass-border: rgba(255, 224, 192, 0.16);
  --glass-border-active: rgba(253, 202, 146, 0.38);
  --accent: #fdca92;
}

.orbit-stage {
  perspective: 1600px;
  perspective-origin: 50% 43%;
  transform-style: preserve-3d;
}

.orbit-card {
  transform-style: preserve-3d;
  background: linear-gradient(155deg, rgba(36, 33, 29, 0.74), var(--glass-bg));
  border: 1px solid var(--glass-border);
  backdrop-filter: blur(22px) saturate(120%);
}
```

```css
.overview-hero { min-height: clamp(620px, 72vw, 820px); overflow: clip; }
.orbit-stage { height: clamp(560px, 64vw, 740px); }
.orbit-card { width: min(590px, 72vw); min-height: 560px; left: 50%; margin-left: min(-295px, -36vw); }

@media (max-width: 900px) {
  .overview-hero { min-height: 650px; }
  .orbit-card { width: min(560px, 78vw); margin-left: min(-280px, -39vw); }
}

@media (max-width: 720px) {
  .overview-hero { min-height: 610px; }
  .orbit-stage { height: 540px; perspective: 1100px; }
  .orbit-card { width: calc(100vw - 2rem); min-height: 500px; margin-left: calc((100vw - 2rem) / -2); }
  .shell-sidebar { top: auto; bottom: 0.6rem; left: 50%; transform: translateX(-50%); }
  .shell-sidebar ul { flex-direction: row; }
}
```

Keep orbit neighbors partially cropped on desktop, retain the tall left glass rail and metadata pills, and ensure the 320 px viewport has no horizontal page overflow.

- [ ] **Step 6: Run the focused visual-foundation tests**

Run: `npm test -- tests/components/foundation.test.tsx tests/components/home-page.test.tsx tests/components/overview-carousel.test.tsx`

Expected: PASS with exact source assets, orbit selectors, and accessibility semantics.

- [ ] **Step 7: Commit the visual system**

```bash
git add web/src/app web/src/components/system web/public/figma web/tests/components/foundation.test.tsx web/tests/components/home-page.test.tsx
git commit -m "feat(web): apply Figma ambient command center system"
```

---

### Task 5: Full verification and blocking design QA

**Files:**
- Create: `design-qa.md`
- Add: `.codex/qa/orbit-desktop.png`
- Add: `.codex/qa/orbit-tablet.png`
- Add: `.codex/qa/orbit-mobile.png`
- Modify when evidence requires it: `web/src/app/globals.css`, `web/src/features/overview/overview-carousel.tsx`, `web/src/features/overview/overview-card.tsx`, `web/src/components/system/ambient-scene.tsx`, `web/src/components/system/command-center-shell.tsx`

**Interfaces:**
- Consumes: the completed four-card scene, the source screenshot, Figma node `13:36`, and the existing Next.js development server.
- Produces: fresh automated verification output, same-state screenshots, a passed `design-qa.md`, and a running local preview.

- [ ] **Step 1: Run the complete automated verification set**

Run: `npm test`

Expected: all Vitest files pass with zero failures.

Run: `npm run lint`

Expected: ESLint exits 0 with no errors.

Run: `npm run typecheck`

Expected: TypeScript exits 0.

Run: `npm run build`

Expected: Next.js production build exits 0.

- [ ] **Step 2: Start the local application and open it in the Codex in-app browser**

Run: `npm run dev -- --hostname 127.0.0.1 --port 3000`

Open `http://127.0.0.1:3000` in the in-app browser. Keep the server running through handoff.

- [ ] **Step 3: Exercise the primary interactions**

Verify click promotion, previous/next rotation, pause/resume, keyboard arrows, pointer drag/swipe, active-card anchor navigation, the judge demo trigger, incident selection, replay action gating, and reset confirmation. Check the browser console after interaction and record whether errors occurred.

- [ ] **Step 4: Capture desktop, tablet, and mobile evidence**

Capture the initial Incident / Trace state at 1440×1024, 900×1024, and 390×844. Save the implementation screenshots under `.codex/qa/`. Use the supplied 1440×1024 target screenshot and Figma frame `13:36` as source truth.

- [ ] **Step 5: Create a side-by-side comparison and write `design-qa.md`**

The report must record source and implementation paths, viewport, CSS size, density, interaction state, full-view evidence, focused card/header/rail evidence, fonts, spacing, colors, image quality, copy, responsiveness, interaction results, console results, comparison history, and `final result: blocked` while any P0/P1/P2 remains.

- [ ] **Step 6: Apply the recorded selector/component fixes for every P0/P1/P2 and repeat capture/comparison**

For each iteration, record the earlier finding, source-visible difference, exact fix, and post-fix screenshot. Stop only when no actionable P0/P1/P2 remains and set `final result: passed`. Leave P3 polish as optional follow-up notes.

- [ ] **Step 7: Re-run the complete verification set after QA fixes**

Run: `npm test && npm run lint && npm run typecheck && npm run build`

Expected: all commands exit 0 after the final visual changes.

- [ ] **Step 8: Commit QA evidence and final fixes**

```bash
git add design-qa.md .codex/qa web/src web/tests web/public web/package.json web/package-lock.json
git commit -m "test(web): verify orbital command center design"
```
