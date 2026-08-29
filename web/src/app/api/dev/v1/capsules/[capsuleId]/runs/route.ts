import { NextResponse } from "next/server";

import { createDevelopmentRun, unavailableFixture } from "../../../../../../../features/replay/development-api";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ capsuleId: string }> },
) {
  const { capsuleId } = await params;
  const body: unknown = await request.json().catch(() => undefined);
  const result = process.env.NODE_ENV === "development" ? createDevelopmentRun(capsuleId, body) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
