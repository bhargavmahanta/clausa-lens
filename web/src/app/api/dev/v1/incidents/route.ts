import { NextResponse } from "next/server";

import { listDevelopmentIncidents, unavailableFixture } from "../../../../../features/replay/development-api";

export async function GET() {
  const result = process.env.NODE_ENV === "development" ? listDevelopmentIncidents() : unavailableFixture();
  return NextResponse.json(result.body, { status: result.status });
}
