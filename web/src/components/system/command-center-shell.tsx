import type { ReactNode } from "react";

export type CommandCenterShellProps = {
  children: ReactNode;
};

const sidebarLinks = [
  { href: "#overview-hero", label: "Overview", icon: "home" },
  { href: "#demo-trigger", label: "Capture trigger", icon: "bolt" },
  { href: "#incident-workspace", label: "Trace and timeline", icon: "nodes" },
  { href: "#replay-lab", label: "Replay workflow", icon: "replay" },
  { href: "#reset-control", label: "Reset controls", icon: "shield" },
] as const;

function SidebarIcon({ icon }: { icon: (typeof sidebarLinks)[number]["icon"] }) {
  switch (icon) {
    case "home":
      return (
        <path d="M12 3l7 6.5V20a1 1 0 01-1 1h-4.5v-5.5h-3V21H6a1 1 0 01-1-1V9.5L12 3z" />
      );
    case "bolt":
      return <path d="M13 2L4 14h6l-1 8 9-12h-6l1-8z" />;
    case "nodes":
      return (
        <>
          <circle cx="5" cy="6" r="2.2" />
          <circle cx="19" cy="6" r="2.2" />
          <circle cx="12" cy="18" r="2.2" />
          <path d="M6.7 7.4l4 8.2M17.3 7.4l-4 8.2" strokeWidth="1.6" />
        </>
      );
    case "replay":
      return <path d="M4 12a8 8 0 018-8 8 8 0 017.4 5M20 12a8 8 0 01-8 8 8 8 0 01-7.4-5M4 4v5h5M20 20v-5h-5" strokeWidth="2" />;
    case "shield":
      return <path d="M12 2l8 3v6c0 5-3.5 9.5-8 11-4.5-1.5-8-6-8-11V5l8-3z" strokeWidth="2" />;
  }
}

export function CommandCenterShell({ children }: CommandCenterShellProps) {
  return (
    <div className="command-center-shell">
      <a className="skip-link" href="#main-content">Skip to main content</a>

      <header className="topbar">
        <a className="brand" href="#main-content" aria-label="CausaLens Command Center home">
          <span className="brand__mark" aria-hidden="true">CL</span>
          <span>
            <strong>CausaLens</strong>
            <small>Command Center</small>
          </span>
        </a>
        <div className="topbar__meta" aria-label="Contract metadata">
          <span className="topbar__pill">Evidence Mode</span>
          <span className="topbar__pill"><code>CONTRACT 1.0</code></span>
        </div>
      </header>

      <nav className="shell-sidebar" aria-label="Workflow sections">
        <ul>
          {sidebarLinks.map((link) => (
            <li key={link.href}>
              <a href={link.href} aria-label={link.label} title={link.label}>
                <svg aria-hidden="true" fill="currentColor" height="20" viewBox="0 0 24 24" width="20">
                  <SidebarIcon icon={link.icon} />
                </svg>
              </a>
            </li>
          ))}
        </ul>
      </nav>

      <main id="main-content" tabIndex={-1}>{children}</main>
      <footer className="footer">
        <span>Controlled replay for instrumented systems</span>
        <span>Evidence over inference</span>
      </footer>
    </div>
  );
}
