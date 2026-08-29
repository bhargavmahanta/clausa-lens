import { NextResponse } from "next/server";

import { resetDevelopmentDemo, unavailableFixture } from "../../../../../../features/replay/development-api";

export async function POST(request: Request) {
  const body: unknown = await request.json().catch(() => undefined);
  const result = process.env.NODE_ENV === "development" ? resetDevelopmentDemo(body) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
