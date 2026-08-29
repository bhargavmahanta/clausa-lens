import { NextResponse } from "next/server";

const ALLOWED_METHODS = new Set(["GET", "POST"]);

function upstreamBase(): string | undefined {
  const configured = process.env.CAUSALENS_CORE_API_URL;
  if (!configured) return undefined;
  return configured.replace(/\/+$/, "");
}

function isSafePathSegment(segment: string): boolean {
  return (
    segment.length > 0 &&
    segment !== "." &&
    segment !== ".." &&
    !segment.includes("/") &&
    !segment.includes("\\") &&
    !segment.includes("%2e") &&
    !segment.includes("%2E") &&
    !/^[a-z][a-z0-9+.-]*:/i.test(segment)
  );
}

function frozenError(status: number, message: string): NextResponse {
  return NextResponse.json(
    { error: { code: "INTERNAL_FAILURE", message, retryable: false, details: {} } },
    { status },
  );
}

async function proxy(
  request: Request,
  context: { params: Promise<{ path: string[] }> },
): Promise<Response> {
  if (!ALLOWED_METHODS.has(request.method)) {
    return frozenError(405, `Method ${request.method} is not proxied to the Core API.`);
  }

  const base = upstreamBase();
  if (!base) {
    return frozenError(503, "CAUSALENS_CORE_API_URL is not configured on the server.");
  }

  const { path } = await context.params;
  if (!Array.isArray(path) || !path.every(isSafePathSegment)) {
    return frozenError(400, "The proxy path is invalid.");
  }

  const search = new URL(request.url).search;
  const upstreamUrl = `${base}/v1/${path.map(encodeURIComponent).join("/")}${search}`;

  const headers: Record<string, string> = { Accept: "application/json" };
  const contentType = request.headers.get("content-type");
  if (contentType) headers["Content-Type"] = contentType;

  let init: RequestInit = { method: request.method, headers };
  if (request.method === "POST") {
    init = { ...init, body: await request.text() };
  }

  try {
    const upstream = await fetch(upstreamUrl, init);
    const body = await upstream.arrayBuffer();
    return new NextResponse(body, {
      status: upstream.status,
      headers: { "Content-Type": upstream.headers.get("content-type") ?? "application/json" },
    });
  } catch {
    return frozenError(502, "The Core API is unreachable from the server proxy.");
  }
}

export const GET = proxy;
export const POST = proxy;
