import { NextResponse } from "next/server";

import { getDevelopmentDiff, unavailableFixture } from "../../../../../../features/replay/development-api";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ diffId: string }> },
) {
  const { diffId } = await params;
  const result = process.env.NODE_ENV === "development" ? getDevelopmentDiff(diffId) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
