import type { ReplayRun } from "../../lib/contracts";

const terminalStatuses = new Set<ReplayRun["status"]>(["COMPLETED", "FAILED", "BLOCKED"]);

export type RunTrackerOptions = {
  getRun: (runId: string) => Promise<ReplayRun>;
  runId: string;
  intervalMs?: number;
  isCancelled?: () => boolean;
  onProgress?: (run: ReplayRun) => void;
  onError?: (error: unknown) => boolean;
};

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function pollRunUntilTerminal(options: RunTrackerOptions): Promise<ReplayRun | undefined> {
  const { getRun, runId, intervalMs = 2000, isCancelled, onProgress, onError } = options;
  let lastRun: ReplayRun | undefined;

  while (!isCancelled?.()) {
    let run: ReplayRun;
    try {
      run = await getRun(runId);
    } catch (error) {
      if (!onError || !onError(error)) {
        throw error;
      }
      await sleep(intervalMs);
      continue;
    }

    lastRun = run;
    onProgress?.(run);

    if (terminalStatuses.has(run.status)) {
      return run;
    }
    if (isCancelled?.()) {
      return run;
    }
    await sleep(intervalMs);
  }

  return lastRun;
}

export function isTerminalRun(run: ReplayRun): boolean {
  return terminalStatuses.has(run.status);
}
