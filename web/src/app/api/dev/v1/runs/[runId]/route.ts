import { NextResponse } from "next/server";

import { getDevelopmentRun, unavailableFixture } from "../../../../../../features/replay/development-api";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ runId: string }> },
) {
  const { runId } = await params;
  const result = process.env.NODE_ENV === "development" ? getDevelopmentRun(runId) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
