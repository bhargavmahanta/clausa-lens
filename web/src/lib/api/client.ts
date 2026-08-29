import type { z } from "zod";

import {
  acceptedEventResponseSchema,
  apiErrorResponseSchema,
  createDiffRequestSchema,
  createRunRequestSchema,
  executionEventSchema,
  incidentDetailResponseSchema,
  incidentListQuerySchema,
  incidentListResponseSchema,
  replayCapsuleSchema,
  replayDiffSchema,
  replayRunSchema,
  resetRequestSchema,
  resetResultSchema,
  type AcceptedEventResponse,
  type APIErrorResponse,
  type CreateDiffRequest,
  type CreateRunRequest,
  type ExecutionEvent,
  type IncidentDetailResponse,
  type IncidentListQuery,
  type IncidentListResponse,
  type ReplayCapsule,
  type ReplayDiff,
  type ReplayRun,
  type ResetRequest,
  type ResetResult,
} from "../contracts";

type FetchImplementation = typeof fetch;

export type CausaLensClientOptions = {
  baseUrl: string;
  fetchImpl?: FetchImplementation;
};

export class ContractDecodeError extends Error {
  readonly resource: string;
  readonly issues: z.core.$ZodIssue[];

  constructor(resource: string, issues: z.core.$ZodIssue[]) {
    super(`The Core API returned an unsupported ${resource} resource.`);
    this.name = "ContractDecodeError";
    this.resource = resource;
    this.issues = issues;
  }
}

export class CausaLensApiError extends Error {
  readonly status: number;
  readonly code: APIErrorResponse["error"]["code"];
  readonly retryable: boolean;
  readonly details: APIErrorResponse["error"]["details"];

  constructor(status: number, apiError: APIErrorResponse["error"]) {
    super(apiError.message);
    this.name = "CausaLensApiError";
    this.status = status;
    this.code = apiError.code;
    this.retryable = apiError.retryable;
    this.details = apiError.details;
  }
}

export class ContractValidationError extends Error {
  readonly resource: string;
  readonly issues: z.core.$ZodIssue[];

  constructor(resource: string, issues: z.core.$ZodIssue[]) {
    super(`${resource} does not satisfy the frozen v1.0 contract.`);
    this.name = "ContractValidationError";
    this.resource = resource;
    this.issues = issues;
  }
}

export type CausaLensClient = {
  acceptEvent(event: ExecutionEvent): Promise<AcceptedEventResponse>;
  listIncidents(query?: IncidentListQuery): Promise<IncidentListResponse>;
  getIncident(incidentId: string): Promise<IncidentDetailResponse>;
  createCapsule(incidentId: string): Promise<ReplayCapsule>;
  createRun(capsuleId: string, request: CreateRunRequest): Promise<ReplayRun>;
  getRun(runId: string): Promise<ReplayRun>;
  createDiff(request: CreateDiffRequest): Promise<ReplayDiff>;
  getDiff(diffId: string): Promise<ReplayDiff>;
  resetDemo(request: ResetRequest): Promise<ResetResult>;
};

export function createCausaLensClient({
  baseUrl,
  fetchImpl = fetch,
}: CausaLensClientOptions): CausaLensClient {
  const normalizedBaseUrl = baseUrl.replace(/\/$/, "");

  function validate<T>(resource: string, schema: z.ZodType<T>, value: unknown): T {
    const decoded = schema.safeParse(value);
    if (!decoded.success) {
      throw new ContractValidationError(resource, decoded.error.issues);
    }
    return decoded.data;
  }

  async function request<T>(path: string, schema: z.ZodType<T>, init?: RequestInit): Promise<T> {
    const hasBody = init?.body !== undefined;
    const response = await fetchImpl(`${normalizedBaseUrl}${path}`, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(hasBody ? { "Content-Type": "application/json" } : {}),
        ...init?.headers,
      },
    });
    const body: unknown = await response.json();

    if (!response.ok) {
      const decodedError = apiErrorResponseSchema.safeParse(body);
      if (!decodedError.success) {
        throw new ContractDecodeError("APIErrorResponse", decodedError.error.issues);
      }
      throw new CausaLensApiError(response.status, decodedError.data.error);
    }

    const decoded = schema.safeParse(body);
    if (!decoded.success) {
      throw new ContractDecodeError(schema.description ?? "API", decoded.error.issues);
    }

    return decoded.data;
  }

  return {
    async acceptEvent(event) {
      const validatedEvent = validate("ExecutionEvent", executionEventSchema, event);
      return request("/v1/events", acceptedEventResponseSchema.describe("AcceptedEventResponse"), {
        method: "POST",
        body: JSON.stringify(validatedEvent),
      });
    },
    async listIncidents(query = {}) {
      const validatedQuery = validate("IncidentListQuery", incidentListQuerySchema, query);
      const search = new URLSearchParams();
      if (validatedQuery.status) search.set("status", validatedQuery.status);
      if (validatedQuery.cursor) search.set("cursor", validatedQuery.cursor);
      if (query.limit !== undefined) search.set("limit", String(validatedQuery.limit));
      const suffix = search.size > 0 ? `?${search.toString()}` : "";
      return request(`/v1/incidents${suffix}`, incidentListResponseSchema.describe("IncidentListResponse"));
    },
    async getIncident(incidentId) {
      return request(
        `/v1/incidents/${encodeURIComponent(incidentId)}`,
        incidentDetailResponseSchema.describe("IncidentDetailResponse"),
      );
    },
    async createCapsule(incidentId) {
      return request(
        `/v1/incidents/${encodeURIComponent(incidentId)}/capsules`,
        replayCapsuleSchema.describe("ReplayCapsule"),
        { method: "POST" },
      );
    },
    async createRun(capsuleId, runRequest) {
      const validatedRequest = validate("CreateRunRequest", createRunRequestSchema, runRequest);
      return request(
        `/v1/capsules/${encodeURIComponent(capsuleId)}/runs`,
        replayRunSchema.describe("ReplayRun"),
        { method: "POST", body: JSON.stringify(validatedRequest) },
      );
    },
    async getRun(runId) {
      return request(`/v1/runs/${encodeURIComponent(runId)}`, replayRunSchema.describe("ReplayRun"));
    },
    async createDiff(diffRequest) {
      const validatedRequest = validate("CreateDiffRequest", createDiffRequestSchema, diffRequest);
      return request("/v1/diffs", replayDiffSchema.describe("ReplayDiff"), {
        method: "POST",
        body: JSON.stringify(validatedRequest),
      });
    },
    async getDiff(diffId) {
      return request(`/v1/diffs/${encodeURIComponent(diffId)}`, replayDiffSchema.describe("ReplayDiff"));
    },
    async resetDemo(resetRequest) {
      const validatedRequest = validate("ResetRequest", resetRequestSchema, resetRequest);
      return request("/v1/demo/reset", resetResultSchema.describe("ResetResult"), {
        method: "POST",
        body: JSON.stringify(validatedRequest),
      });
    },
  };
}
