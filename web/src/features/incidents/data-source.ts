export type IncidentDataSource = {
  baseUrl: string;
  mode: "core" | "fixture";
};

export function resolveIncidentDataSource({
  configuredBaseUrl,
  isDevelopment,
}: {
  configuredBaseUrl?: string;
  isDevelopment: boolean;
}): IncidentDataSource {
  if (configuredBaseUrl) {
    return { baseUrl: configuredBaseUrl.replace(/\/$/, ""), mode: "core" };
  }

  if (isDevelopment) {
    return { baseUrl: "/api/dev", mode: "fixture" };
  }

  return { baseUrl: "http://localhost:8080", mode: "core" };
}
