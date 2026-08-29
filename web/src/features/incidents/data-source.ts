export type IncidentDataSource = {
  baseUrl: string;
  mode: "core" | "fixture";
};

export function resolveIncidentDataSource({
  isDevelopment,
}: {
  isDevelopment: boolean;
}): IncidentDataSource {
  if (isDevelopment) {
    return { baseUrl: "/api/dev", mode: "fixture" };
  }

  return { baseUrl: "/v1", mode: "core" };
}
