# Orbital Command Center Frontend Design

## Purpose

Rebuild the CausaLens Command Center around a premium, Figma-derived dark environment and a genuinely spatial overview. The overview must make the four investigation outputs understandable at a glance while preserving every existing backend-connected workflow below it.

The primary interaction is a tilted, elliptical 3D orbit. Selecting any background card rotates the whole system coherently so the chosen card travels forward along a curved surface while the other cards advance through the same shared orbit.

## Product Boundary

The frontend continues to display decoded Core API evidence; it does not invent incident findings, replay status, oracle results, isolation verdicts, or diffs. Existing incident loading, golden-demo triggering, capsule compilation, baseline replay, what-if replay, diff creation, polling, error states, and reset confirmation remain functional.

The orbit is an overview and navigation surface. Detailed controls and evidence remain in the existing investigation and replay workspace below, with direct links from the active overview card to the corresponding section.

## Selected Approach

Use normal React DOM cards placed in a CSS 3D scene and animate a shared orbit parameter with GSAP. Each card's position is derived continuously from an angle on a tilted ellipse rather than from a small set of unrelated transform presets.

This approach is preferred over the current independent Motion transforms because it guarantees a coherent curved trajectory and permits synchronized depth, rotation, focus, lighting, and parallax. It is preferred over Three.js because the cards contain accessible text, buttons, status values, and navigation; keeping them in the DOM preserves rendering and interaction quality.

The `motion` dependency may remain for existing unrelated UI, but the overview scene will use GSAP. Three.js, React Three Fiber, Zustand, and TanStack Query are not required for this scope.

## Information Architecture

The orbit contains exactly four primary cards:

1. **Incident / Trace** — selected incident, failure oracle, Gateway → Checkout → Payment → Ledger path, and capture state.
2. **Replay Capsule** — capsule readiness, integrity/safety framing, and the next capsule action.
3. **Replay** — baseline and what-if state, current outcomes, and the next replay action.
4. **Diff** — effect delta, oracle change, first meaningful divergence readiness, and workflow completion.

Only the front card exposes full detail and its call to action. Background cards show their identity and compact status, remain clickable, and hide their inactive content from the accessibility tree.

## Orbit Geometry and Animation

The scene uses a perspective container and an invisible tilted ellipse. A normalized orbit angle drives each card's position:

- horizontal position follows the ellipse cosine;
- vertical position follows the ellipse sine and is rotated to create a diagonal ring;
- depth follows the sine/cosine phase so cards pass visibly behind the focus plane;
- scale, opacity, blur, border intensity, and shadow interpolate from depth;
- z-index is derived from depth and updated during the animation;
- card orientation subtly follows the ellipse tangent and straightens as it reaches the front.

Selecting a card calculates the shortest angular delta that brings it to the front. GSAP animates one shared orbit value for roughly 0.9 seconds with `power3.inOut`, followed by a restrained settling emphasis on the promoted card. All card transforms are recomputed on each animation update, which prevents discontinuous straight-line movement between slots.

Interaction methods share the same state transition:

- click/tap a background card;
- previous and next controls;
- horizontal pointer drag or touch swipe with a threshold and snap to the nearest card;
- left and right arrow keys when the scene is focused;
- automatic rotation only while the scene is not hovered, focused, dragging, or manually paused.

An `aria-live` region announces the focused card. Only front-card content is interactive. Reduced-motion mode disables continuous orbit travel, parallax, auto-rotation, and flicker, and changes focus instantly with a short opacity transition.

## Visual System

The Figma file is the source for the component imagery and visual proportions. Assets will be downloaded from the relevant Figma nodes rather than redrawn when a source asset exists.

The scene combines:

- near-black room/background;
- translucent charcoal glass with warm low-contrast borders;
- a restrained amber highlight and cursor-responsive glass sheen;
- layered stone, monolith, plant, ring, and sphere imagery;
- one active-card hierarchy with stronger contrast and readable evidence;
- background cards with lower opacity, shallow blur, and reduced content density.

The illuminated ring has a separate blurred glow layer. Its intensity receives irregular ambient pulses and rare micro-flickers; the image itself stays stable. Pointer parallax moves background layers by small depth-specific distances and never shifts readable content enough to interfere with selection.

The header and sidebar will be refined toward the supplied reference: compact brand block, glass metadata controls, vertical desktop rail, and a mobile bottom rail. Existing anchors and accessible labels remain intact.

## Component Boundaries

- `OverviewCarousel` owns accessible scene state, pause rules, focus promotion, and input coordination.
- Orbit geometry helpers are pure functions that convert stage index and orbit angle into render transforms.
- `OverviewCard` renders the compact/front variants without owning backend truth.
- Ambient scene markup owns Figma assets, glow layers, and pointer parallax independently from workflow state.
- `CommandCenter` continues to derive all card content from its current incident/replay reducer and API client.
- The existing investigation dashboard and replay workspace remain the authoritative locations for full actions and evidence.

## Responsive Behavior

Desktop and wide tablet layouts show the full 3D orbit with neighboring cards partially visible. Smaller tablets reduce ellipse radii and scenery intensity. Mobile presents one full card plus shallow edge previews, retains swipe and previous/next navigation, moves the sidebar to a bottom rail, and avoids horizontal page overflow.

The hero height uses responsive bounds rather than a fixed desktop-only canvas. Text inside the front card remains readable at 320 px viewport width, and controls meet a minimum 44 px target size.

## Error and Loading States

Cards must express existing idle, loading, ready, active, terminal, and error states using the current reducer outputs. Missing data produces truthful pending copy rather than fabricated evidence. Animation failures must not block the workflow: the DOM remains usable at its last stable selection, and direct section links remain available.

## Testing

Test-first coverage will include:

- continuous orbit geometry and depth ordering;
- shortest-path selection and wrap-around behavior;
- exactly one front card for every focus state;
- accessible front/background content behavior;
- pause, keyboard, and reduced-motion semantics;
- preservation of the four backend-derived overview objects;
- existing incident, replay, reset, and command-center regression tests.

Fresh verification requires the complete Vitest suite, ESLint, TypeScript typecheck, and Next.js production build.

## Visual QA

After implementation, capture the running application at desktop, tablet, and mobile widths. Compare the desktop result with the supplied target screenshot and the Figma node for:

- scene composition and depth;
- active-card size and prominence;
- partial visibility and curvature of background cards;
- glass color, blur, border, and warm highlights;
- asset placement and glow intensity;
- header/sidebar proportions;
- absence of clipping, overflow, illegible data, or inaccessible controls.

The final handoff requires a visual QA record with a passed result and a running local preview.
