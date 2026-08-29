import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import HomePage from "../../src/app/page";

describe("Command Center home", () => {
  it("introduces the evidence workflow without fabricating domain status", () => {
    const markup = renderToStaticMarkup(<HomePage />);

    expect(markup).toContain("CausaLens");
    expect(markup).toContain("Command Center");
    expect(markup).toContain("Waiting for Core API");
    expect(markup).toContain("Capture");
    expect(markup).toContain("Trace");
    expect(markup).toContain("Diff");
    expect(markup).not.toContain("REPRODUCED");
    expect(markup).not.toContain("Isolation verified");
  });
});
