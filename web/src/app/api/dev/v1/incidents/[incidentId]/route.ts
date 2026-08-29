import { NextResponse } from "next/server";

import { getDevelopmentIncident, unavailableFixture } from "../../../../../../features/replay/development-api";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ incidentId: string }> },
) {
  const { incidentId } = await params;
  const result = process.env.NODE_ENV === "development" ? getDevelopmentIncident(incidentId) : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
