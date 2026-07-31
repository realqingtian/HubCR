import { healthResponseSchema, type HealthResponse } from "./schemas";

const apiBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export async function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  const response = await fetch(`${apiBaseURL}/api/v1/health/ready`, { signal });
  if (!response.ok) {
    throw new Error(`HubCR API returned HTTP ${response.status}`);
  }

  return healthResponseSchema.parse(await response.json());
}
