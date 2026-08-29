// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetResult } from "../fixtures/golden-contracts";

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function findButtonByText(container: HTMLElement, text: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll("button")).find(
    (button) => button.textContent?.trim() === text,
  );
}

describe("reset confirmation dialog", () => {
  let container: HTMLDivElement;
  let root: Root | undefined;
  let resetRequests: number;

  async function respondToFetch(input: RequestInfo | URL): Promise<Response> {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.includes("/v1/incidents")) {
      return new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    if (url.includes("/v1/demo/reset")) {
      resetRequests += 1;
      return new Response(JSON.stringify(resetResult), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response(
      JSON.stringify({
        error: { code: "INTERNAL_FAILURE", message: `Unexpected request ${url}`, retryable: false, details: {} },
      }),
      { status: 404, headers: { "Content-Type": "application/json" } },
    );
  }

  beforeEach(() => {
    resetRequests = 0;
    vi.stubGlobal("fetch", vi.fn(respondToFetch));
    container = document.createElement("div");
    document.body.appendChild(container);
  });

  afterEach(async () => {
    await act(async () => {
      root?.unmount();
    });
    container.remove();
    vi.unstubAllGlobals();
  });

  it("opens the dialog on first click, sends no reset, then sends exactly one on confirmation", async () => {
    const { CommandCenter } = await import("../../src/features/command-center/command-center");
    await act(async () => {
      root = createRoot(container);
      root.render(<CommandCenter />);
    });
    await act(flush);

    const trigger = findButtonByText(container, "Reset demo workflow");
    expect(trigger).toBeDefined();
    expect(container.querySelector('[role="dialog"]')).toBeNull();

    await act(async () => {
      trigger!.click();
    });

    const dialog = container.querySelector('[role="dialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.textContent).toContain("Confirm demo reset");
    expect(resetRequests).toBe(0);

    const confirm = findButtonByText(dialog as HTMLElement, "Reset demo");
    expect(confirm).toBeDefined();

    await act(async () => {
      confirm!.click();
    });
    await act(flush);

    expect(resetRequests).toBe(1);
    expect(container.textContent).toContain("Demo reset complete");
    expect(container.querySelector('[role="dialog"]')).toBeNull();
  });

  it("sends no reset request when the dialog is cancelled", async () => {
    const { CommandCenter } = await import("../../src/features/command-center/command-center");
    await act(async () => {
      root = createRoot(container);
      root.render(<CommandCenter />);
    });
    await act(flush);

    await act(async () => {
      findButtonByText(container, "Reset demo workflow")!.click();
    });
    const dialog = container.querySelector('[role="dialog"]') as HTMLElement;
    expect(dialog).not.toBeNull();

    await act(async () => {
      findButtonByText(dialog, "Cancel")!.click();
    });

    expect(resetRequests).toBe(0);
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(findButtonByText(container, "Reset demo workflow")).toBeDefined();
  });
});
