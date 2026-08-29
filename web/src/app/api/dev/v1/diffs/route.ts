import { NextResponse } from "next/server";

import { createDevelopmentDiff, unavailableFixture } from "../../../../../features/replay/development-api";

export async function POST(request: Request) {
  const body: unknown = await request.json().catch(() => undefined);
  const result = process.env.NODE_ENV === "development" ? createDevelopmentDiff(body) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
