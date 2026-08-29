import type {
  ReplayCapsule,
  ReplayDiff,
  ReplayRun,
  ResetResult,
} from "../../lib/contracts";

export type FrontendError = {
  code?: string;
  message: string;
  retryable?: boolean;
};

export type ResourceStage<T> =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ready"; value: T }
  | { status: "error"; error: FrontendError };

export type ResetStage =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "submitting" }
  | { status: "completed"; result: ResetResult }
  | { status: "failed"; error: FrontendError };

export type CommandCenterReplayState = {
  capsule: ResourceStage<ReplayCapsule>;
  baseline: ResourceStage<ReplayRun>;
  whatIf: ResourceStage<ReplayRun>;
  diff: ResourceStage<ReplayDiff>;
  reset: ResetStage;
};

export const initialCommandCenterReplayState: CommandCenterReplayState = {
  capsule: { status: "idle" },
  baseline: { status: "idle" },
  whatIf: { status: "idle" },
  diff: { status: "idle" },
  reset: { status: "idle" },
};

export type CommandCenterReplayAction =
  | { type: "resourceLoading"; resource: "capsule" | "baseline" | "whatIf" | "diff" }
  | { type: "resourceReady"; resource: "capsule"; value: ReplayCapsule }
  | { type: "resourceReady"; resource: "baseline" | "whatIf"; value: ReplayRun }
  | { type: "resourceReady"; resource: "diff"; value: ReplayDiff }
  | { type: "resourceFailed"; resource: "capsule" | "baseline" | "whatIf" | "diff"; error: FrontendError }
  | { type: "incidentChanged" }
  | { type: "resetConfirmationOpened" }
  | { type: "resetConfirmationClosed" }
  | { type: "resetStarted" }
  | { type: "resetSucceeded"; result: ResetResult }
  | { type: "resetFailed"; error: FrontendError };

export type ReplayGenerationToken = {
  issue: () => () => boolean;
  invalidate: () => void;
};

export function createGenerationToken(): ReplayGenerationToken {
  let current = 0;
  return {
    issue: () => {
      const generation = current;
      return () => generation === current;
    },
    invalidate: () => {
      current += 1;
    },
  };
}

export function reduceCommandCenterReplay(
  state: CommandCenterReplayState,
  action: CommandCenterReplayAction,
): CommandCenterReplayState {
  switch (action.type) {
    case "resourceLoading":
      return { ...state, [action.resource]: { status: "loading" } };
    case "resourceReady":
      return { ...state, [action.resource]: { status: "ready", value: action.value } };
    case "resourceFailed":
      return { ...state, [action.resource]: { status: "error", error: action.error } };
    case "incidentChanged":
      return state.capsule.status === "idle" &&
        state.baseline.status === "idle" &&
        state.whatIf.status === "idle" &&
        state.diff.status === "idle"
        ? state
        : {
            ...state,
            capsule: { status: "idle" },
            baseline: { status: "idle" },
            whatIf: { status: "idle" },
            diff: { status: "idle" },
          };
    case "resetConfirmationOpened":
      return { ...state, reset: { status: "confirming" } };
    case "resetConfirmationClosed":
      return { ...state, reset: { status: "idle" } };
    case "resetStarted":
      return state.reset.status === "confirming"
        ? { ...state, reset: { status: "submitting" } }
        : state;
    case "resetSucceeded":
      return {
        ...initialCommandCenterReplayState,
        reset: { status: "completed", result: action.result },
      };
    case "resetFailed":
      return { ...state, reset: { status: "failed", error: action.error } };
    default:
      return state;
  }
}
