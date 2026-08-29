import type { ReactNode } from "react";

export type CommandCenterShellProps = {
  children: ReactNode;
};

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
          <span>Evidence mode</span>
          <code>CONTRACT 1.0</code>
        </div>
      </header>
      <main id="main-content" tabIndex={-1}>{children}</main>
      <footer className="footer">
        <span>Controlled replay for instrumented systems</span>
        <span>Evidence over inference</span>
      </footer>
    </div>
  );
}
