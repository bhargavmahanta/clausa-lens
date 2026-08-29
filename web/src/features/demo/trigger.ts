import type { Incident, IncidentListQuery } from "../../lib/contracts";

export const goldenCheckoutBody = {
  checkout_id: "checkout-8271",
  amount_minor: 4999,
  currency: "INR",
} as const;

export type DemoCheckoutTrace = {
  traceId: string;
  executionId: string;
};

export function findIncidentByTrace(
  incidents: Incident[],
  trace: DemoCheckoutTrace,
): Incident | undefined {
  return incidents.find(
    (incident) => incident.trace_id === trace.traceId && incident.execution_id === trace.executionId,
  );
}

export type PollForIncidentOptions = {
  listIncidents: (query?: IncidentListQuery) => Promise<{ items: Incident[] }>;
  trace: DemoCheckoutTrace;
  intervalMs?: number;
  maxAttempts?: number;
  isCancelled?: () => boolean;
};

export async function pollForIncident(
  options: PollForIncidentOptions,
): Promise<Incident | undefined> {
  const { listIncidents, trace, intervalMs = 500, maxAttempts = 40, isCancelled } = options;

  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    if (isCancelled?.()) return undefined;
    try {
      const response = await listIncidents({ limit: 100 });
      const incident = findIncidentByTrace(response.items, trace);
      if (incident) return incident;
    } catch {
      // Detection can lag the trigger; the attempt budget bounds retrying.
    }
    if (isCancelled?.()) return undefined;
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }

  return undefined;
}

export async function requestDemoCheckout(fetchImpl: typeof fetch): Promise<DemoCheckoutTrace> {
  const response = await fetchImpl("/api/demo/checkout", {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  const body: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    const apiError = (body as { error?: { message?: string } } | undefined)?.error;
    throw new Error(apiError?.message ?? "The faulted checkout trigger failed.");
  }
  const trace = body as { trace_id?: unknown; execution_id?: unknown } | undefined;
  if (typeof trace?.trace_id !== "string" || typeof trace?.execution_id !== "string") {
    throw new Error("The gateway returned an unusable checkout response.");
  }
  return { traceId: trace.trace_id, executionId: trace.execution_id };
}
