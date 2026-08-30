"use client";

import { gsap } from "gsap";
import Image from "next/image";
import { useLayoutEffect, useRef } from "react";

export function AmbientScene() {
  const sceneRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const root = sceneRef.current;
    if (!root) return;

    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    if (media.matches) return;

    const handlePointerMove = (event: globalThis.PointerEvent) => {
      const horizontal = event.clientX / window.innerWidth - 0.5;
      const vertical = event.clientY / window.innerHeight - 0.5;
      root.style.setProperty("--room-x", `${horizontal * -10}px`);
      root.style.setProperty("--room-y", `${vertical * -8}px`);
      root.style.setProperty("--glow-x", `${horizontal * 18}px`);
      root.style.setProperty("--glow-y", `${vertical * 14}px`);
    };

    const context = gsap.context(() => {
      gsap
        .timeline({ repeat: -1, repeatDelay: 3.7 })
        .to(".ambient-ring-glow", {
          opacity: 0.9,
          scale: 1.025,
          duration: 1.9,
          ease: "sine.inOut",
        })
        .to(".ambient-ring-glow", { opacity: 0.69, duration: 0.075 })
        .to(".ambient-ring-glow", { opacity: 1, duration: 0.115 })
        .to(".ambient-ring-glow", {
          opacity: 0.84,
          scale: 0.985,
          duration: 2.25,
          ease: "sine.inOut",
        });
    }, root);

    window.addEventListener("pointermove", handlePointerMove, { passive: true });
    return () => {
      window.removeEventListener("pointermove", handlePointerMove);
      context.revert();
    };
  }, []);

  return (
    <div className="ambient-scene" aria-hidden="true" ref={sceneRef}>
      <Image
        alt=""
        className="ambient-room"
        fill
        priority
        sizes="100vw"
        src="/figma/command-center-room.png"
        unoptimized
      />
      <span className="ambient-ring-glow" />
      <span className="ambient-vignette" />
    </div>
  );
}
