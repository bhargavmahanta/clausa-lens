import { NextResponse } from "next/server";

import { goldenCheckoutBody } from "../../../../features/demo/trigger";

function frozenError(status: number, message: string): NextResponse {
  return NextResponse.json(
    { error: { code: "INTERNAL_FAILURE", message, retryable: false, details: {} } },
    { status },
  );
}

export async function POST(): Promise<Response> {
  const configured = process.env.CAUSALENS_GATEWAY_URL;
  if (!configured) {
    return frozenError(503, "CAUSALENS_GATEWAY_URL is not configured on the server.");
  }
  const gatewayUrl = `${configured.replace(/\/+$/, "")}/checkout`;

  try {
    const upstream = await fetch(gatewayUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(goldenCheckoutBody),
    });
    if (!upstream.ok) {
      return frozenError(502, "The gateway rejected the faulted checkout trigger.");
    }
    const body: unknown = await upstream.json();
    const trace = body as { trace_id?: unknown; execution_id?: unknown } | undefined;
    if (typeof trace?.trace_id !== "string" || typeof trace?.execution_id !== "string") {
      return frozenError(502, "The gateway returned an unusable checkout response.");
    }
    return NextResponse.json(body, { status: 200 });
  } catch {
    return frozenError(502, "The gateway is unreachable from the server.");
  }
}
