import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import HomePage from "../../src/app/page";

describe("Command Center home", () => {
  it("opens the C2 incident workspace without fabricating domain status", () => {
    const markup = renderToStaticMarkup(<HomePage />);

    expect(markup).toContain("CausaLens");
    expect(markup).toContain("Command Center");
    expect(markup).toContain("Incident analysis");
    expect(markup).toContain("Loading incident evidence");
    expect(markup).toContain("Capture → Trace");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).not.toContain("Isolation verified");
  });
});
