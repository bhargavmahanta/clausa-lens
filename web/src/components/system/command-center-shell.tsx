import Image from "next/image";
import type { ReactNode } from "react";

export type CommandCenterShellProps = {
  children: ReactNode;
};

const sidebarLinks = [
  { href: "#overview-hero", label: "Overview", icon: "/figma/icons/home.png" },
  { href: "#demo-trigger", label: "Capture trigger", icon: "/figma/icons/clock.png" },
  { href: "#incident-workspace", label: "Trace and timeline", icon: "/figma/icons/search.png" },
  { href: "#replay-lab", label: "Replay workflow", icon: "/figma/icons/grid.png" },
  { href: "#reset-control", label: "Reset controls", icon: "/figma/icons/shield.png" },
] as const;

export function CommandCenterShell({ children }: CommandCenterShellProps) {
  return (
    <div className="command-center-shell">
      <a className="skip-link" href="#main-content">
        Skip to main content
      </a>

      <header className="topbar">
        <a className="brand" href="#main-content" aria-label="CausaLens Command Center home">
          <Image
            alt=""
            className="brand__mark"
            height={57}
            priority
            src="/figma/causalens-logo.png"
            unoptimized
            width={49}
          />
          <span>
            <strong>CausaLens</strong>
            <small>Command Center</small>
          </span>
        </a>

        <div className="topbar__actions">
          <div className="topbar__meta" aria-label="Contract metadata">
            <button className="topbar__pill" type="button">
              <span>Evidence Mode</span>
              <Image alt="" height={6} src="/figma/icons/chevron.png" unoptimized width={11} />
            </button>
            <button className="topbar__pill" type="button">
              <span>Contract 1.0</span>
              <Image alt="" height={6} src="/figma/icons/chevron.png" unoptimized width={11} />
            </button>
          </div>
          <button className="topbar__icon" aria-label="Notifications" type="button">
            <Image alt="" height={29} src="/figma/icons/notifications.png" unoptimized width={24} />
          </button>
          <button className="topbar__icon" aria-label="Profile" type="button">
            <Image alt="" height={31} src="/figma/icons/profile.png" unoptimized width={31} />
          </button>
        </div>
      </header>

      <nav className="shell-sidebar" aria-label="Workflow sections">
        <div className="shell-sidebar__logo" aria-hidden="true">
          <Image alt="" height={57} src="/figma/causalens-logo.png" unoptimized width={49} />
        </div>
        <ul>
          {sidebarLinks.map((link) => (
            <li key={link.href}>
              <a href={link.href} aria-label={link.label} title={link.label}>
                <Image alt="" height={24} src={link.icon} unoptimized width={24} />
              </a>
            </li>
          ))}
        </ul>
        <span className="shell-sidebar__status" aria-label="Replay environment protected">
          <Image alt="" height={22} src="/figma/icons/shield.png" unoptimized width={22} />
        </span>
      </nav>

      <main id="main-content" tabIndex={-1}>
        {children}
      </main>
      <footer className="footer">
        <span>Controlled replay for instrumented systems</span>
        <span>Evidence over inference</span>
      </footer>
    </div>
  );
}
