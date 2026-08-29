import { NextResponse } from "next/server";

import { compileDevelopmentCapsule, unavailableFixture } from "../../../../../../../features/replay/development-api";

export async function POST(
  _request: Request,
  { params }: { params: Promise<{ incidentId: string }> },
) {
  const { incidentId } = await params;
  const result = process.env.NODE_ENV === "development" ? compileDevelopmentCapsule(incidentId) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
