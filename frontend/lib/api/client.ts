import type { z } from "zod";
import {
  artifactSecuritySchema,
  artifactListSchema,
  artifactDetailSchema,
  errorEnvelopeSchema,
  healthResponseSchema,
  loginResponseSchema,
  membershipListSchema,
  organizationListSchema,
  organizationSchema,
  repositoryDetailSchema,
  repositoryListSchema,
  repositorySchema,
  tagListSchema,
  tagSchema,
  userSchema,
  type Artifact,
  type ArtifactSecurity,
  type ArtifactList,
  type FieldError,
  type HealthResponse,
  type LoginResponse,
  type MembershipList,
  type Organization,
  type OrganizationList,
  type OrganizationRole,
  type Repository,
  type RepositoryDetail,
  type RepositoryList,
  type RepositoryVisibility,
  type Tag,
  type TagList,
  type User,
} from "./schemas";

const apiBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

export class APIError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: FieldError[];
  readonly requestID?: string;

  constructor(status: number, code: string, message: string, fields: FieldError[] = [], requestID?: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.requestID = requestID;
  }
}

type PageInput = Readonly<{ limit?: number; cursor?: string }>;

function pageQuery(input: PageInput = {}): string {
  const query = new URLSearchParams();
  if (input.limit !== undefined) query.set("limit", String(input.limit));
  if (input.cursor) query.set("cursor", input.cursor);
  const value = query.toString();
  return value ? `?${value}` : "";
}

async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  init: RequestInit = {},
): Promise<T> {
  const response = await fetch(`${apiBaseURL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(init.body === undefined ? {} : { "Content-Type": "application/json" }),
      ...init.headers,
    },
  });
  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => undefined);
    const parsed = errorEnvelopeSchema.safeParse(payload);
    if (parsed.success) {
      throw new APIError(
        response.status,
        parsed.data.error.code,
        parsed.data.error.message,
        parsed.data.error.fields ?? [],
        parsed.data.request_id,
      );
    }
    throw new APIError(response.status, "invalid_response", `HubCR API returned HTTP ${response.status}`);
  }
  return schema.parse(await response.json());
}

async function requestEmpty(path: string, init: RequestInit): Promise<void> {
  const response = await fetch(`${apiBaseURL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(init.body === undefined ? {} : { "Content-Type": "application/json" }),
      ...init.headers,
    },
  });
  if (!response.ok) {
    const payload: unknown = await response.json().catch(() => undefined);
    const parsed = errorEnvelopeSchema.safeParse(payload);
    if (parsed.success) {
      throw new APIError(
        response.status,
        parsed.data.error.code,
        parsed.data.error.message,
        parsed.data.error.fields ?? [],
        parsed.data.request_id,
      );
    }
    throw new APIError(response.status, "invalid_response", `HubCR API returned HTTP ${response.status}`);
  }
}

export async function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  return request("/api/v1/health/ready", healthResponseSchema, { signal });
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  return request("/api/v1/auth/login", loginResponseSchema, {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function getCurrentUser(signal?: AbortSignal): Promise<User> {
  return request("/api/v1/auth/me", userSchema, { signal });
}

export async function logout(): Promise<void> {
  return requestEmpty("/api/v1/auth/logout", { method: "POST" });
}

export async function listOrganizations(input?: PageInput): Promise<OrganizationList> {
  return request(`/api/v1/organizations${pageQuery(input)}`, organizationListSchema);
}

export async function createOrganization(name: string, description: string): Promise<Organization> {
  return request("/api/v1/organizations", organizationSchema, {
    method: "POST",
    body: JSON.stringify({ name, description }),
  });
}

export async function listMembers(organizationID: string, input?: PageInput): Promise<MembershipList> {
  return request(
    `/api/v1/organizations/${encodeURIComponent(organizationID)}/members${pageQuery(input)}`,
    membershipListSchema,
  );
}

export async function addMember(
  organizationID: string,
  userID: string,
  role: OrganizationRole,
): Promise<void> {
  return requestEmpty(`/api/v1/organizations/${encodeURIComponent(organizationID)}/members`, {
    method: "POST",
    body: JSON.stringify({ user_id: userID, role }),
  });
}

export async function listRepositories(namespace: string, input?: PageInput): Promise<RepositoryList> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories${pageQuery(input)}`,
    repositoryListSchema,
  );
}

export async function getRepository(namespace: string, name: string): Promise<RepositoryDetail> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(name)}`,
    repositoryDetailSchema,
  );
}

export async function listArtifacts(namespace: string, repository: string, input?: PageInput): Promise<ArtifactList> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(repository)}/artifacts${pageQuery(input)}`,
    artifactListSchema,
  );
}

export async function getArtifact(namespace: string, repository: string, digest: string): Promise<Artifact> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(repository)}/artifacts/${encodeURIComponent(digest)}`,
    artifactDetailSchema,
  );
}

export async function getArtifactSecurity(
  namespace: string,
  repository: string,
  digest: string,
): Promise<ArtifactSecurity> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(repository)}/artifacts/${encodeURIComponent(digest)}/security`,
    artifactSecuritySchema,
  );
}

export async function listTags(namespace: string, repository: string, input?: PageInput): Promise<TagList> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(repository)}/tags${pageQuery(input)}`,
    tagListSchema,
  );
}

export async function getTag(namespace: string, repository: string, tag: string): Promise<Tag> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(repository)}/tags/${encodeURIComponent(tag)}`,
    tagSchema,
  );
}

export async function createRepository(
  namespace: string,
  name: string,
  visibility: RepositoryVisibility,
  description: string,
): Promise<Repository> {
  return request(`/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories`, repositorySchema, {
    method: "POST",
    body: JSON.stringify({ name, visibility, description }),
  });
}

export async function updateRepository(
  namespace: string,
  name: string,
  input: Readonly<{ visibility?: RepositoryVisibility; description?: string }>,
): Promise<Repository> {
  return request(
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/repositories/${encodeURIComponent(name)}`,
    repositorySchema,
    { method: "PATCH", body: JSON.stringify(input) },
  );
}

export type {
  Artifact,
  ArtifactSecurity,
  SecurityResult,
  SignatureEvidence,
  ArtifactList,
  LoginResponse,
  ManifestDescriptor,
  Organization,
  OrganizationList,
  OrganizationRole,
  Repository,
  RepositoryCapabilities,
  RepositoryDetail,
  RepositoryList,
  RepositoryVisibility,
  Tag,
  TagList,
  User,
} from "./schemas";
