import { z } from "zod";

const idSchema = z.string().uuid();
const timestampSchema = z.iso.datetime({ offset: true });
export const namespaceNameSchema = z
  .string()
  .min(1)
  .max(64)
  .regex(/^[a-z0-9]+(?:[._-][a-z0-9]+)*$/);

export const healthResponseSchema = z.object({
  status: z.literal("ok"),
});

export const fieldErrorSchema = z.object({
  field: z.string(),
  message: z.string(),
});

export const errorEnvelopeSchema = z.object({
  error: z.object({
    code: z.enum([
      "invalid_request",
      "validation_failed",
      "not_found",
      "method_not_allowed",
      "authentication_failed",
      "rate_limited",
      "forbidden",
      "conflict",
      "internal_error",
    ]),
    message: z.string(),
    fields: z.array(fieldErrorSchema).optional(),
  }),
  request_id: z.string(),
});

export const userSchema = z.object({
  id: idSchema,
  username: z.string().min(1).max(64),
  personal_namespace: namespaceNameSchema,
  created_at: timestampSchema,
});

export const loginResponseSchema = z.object({
  user: userSchema,
  expires_at: timestampSchema,
});

export const pageMetaSchema = z.object({
  limit: z.number().int().min(1).max(100),
  next_cursor: z.string().optional(),
});

export const organizationRoleSchema = z.enum(["OWNER", "ADMIN", "WRITER", "READER"]);

export const organizationSchema = z.object({
  id: idSchema,
  namespace: namespaceNameSchema,
  description: z.string(),
  created_by_user_id: idSchema,
  created_at: timestampSchema,
  updated_at: timestampSchema,
});

export const organizationListSchema = z.object({
  items: z.array(organizationSchema),
  meta: pageMetaSchema,
});

export const membershipSchema = z.object({
  user_id: idSchema,
  role: organizationRoleSchema,
  added_by_user_id: idSchema,
  created_at: timestampSchema,
  updated_at: timestampSchema,
});

export const membershipListSchema = z.object({
  items: z.array(membershipSchema),
  meta: pageMetaSchema,
});

export const repositoryVisibilitySchema = z.enum(["PUBLIC", "PRIVATE"]);

export const repositorySchema = z.object({
  id: idSchema,
  namespace: namespaceNameSchema,
  name: namespaceNameSchema,
  visibility: repositoryVisibilitySchema,
  description: z.string().max(1024),
  created_by_user_id: idSchema,
  visibility_updated_by_user_id: idSchema,
  visibility_updated_at: timestampSchema,
  created_at: timestampSchema,
  updated_at: timestampSchema,
});

export const repositoryCapabilitiesSchema = z.object({
  can_pull: z.boolean(),
  can_push: z.boolean(),
});

export const repositoryDetailSchema = repositorySchema.extend({
  capabilities: repositoryCapabilitiesSchema,
});

export const repositoryListSchema = z.object({
  items: z.array(repositorySchema),
  meta: pageMetaSchema,
});

export const artifactDigestSchema = z.string().regex(/^sha256:[0-9a-f]{64}$/);
export const artifactKindSchema = z.enum(["MANIFEST", "INDEX"]);
export const artifactPlatformSchema = z.object({
  os: z.string().min(1).max(64),
  architecture: z.string().min(1).max(64),
  variant: z.string().min(1).max(64).optional(),
});
export const manifestDescriptorSchema = z.object({
  position: z.number().int().min(0),
  digest: artifactDigestSchema,
  media_type: z.string().min(1).max(255).optional(),
  size_bytes: z.number().int().min(0).optional(),
  platform: artifactPlatformSchema.optional(),
});
export const artifactSchema = z.object({
  digest: artifactDigestSchema,
  kind: artifactKindSchema,
  media_type: z.string().min(1).max(255).optional(),
  size_bytes: z.number().int().min(0).optional(),
  source_created_at: timestampSchema.optional(),
  descriptors_complete: z.boolean(),
  discovered_at: timestampSchema,
  updated_at: timestampSchema,
  manifests: z.array(manifestDescriptorSchema).optional(),
});
export const artifactDetailSchema = artifactSchema.superRefine((artifact, context) => {
  if (artifact.kind === "INDEX" && artifact.descriptors_complete && artifact.manifests === undefined) {
    context.addIssue({ code: "custom", message: "a complete Index must include manifests" });
  }
  if ((artifact.kind !== "INDEX" || !artifact.descriptors_complete) && artifact.manifests !== undefined) {
    context.addIssue({ code: "custom", message: "manifests require a complete Index" });
  }
});
export const artifactListSchema = z.object({
  items: z.array(artifactSchema),
  meta: pageMetaSchema,
});
export const tagNameSchema = z.string().min(1).max(128).regex(/^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$/);
export const tagSchema = z.object({
  name: tagNameSchema,
  digest: artifactDigestSchema,
  created_at: timestampSchema,
  updated_at: timestampSchema,
  artifact: artifactSchema.optional(),
});
export const tagListSchema = z.object({
  items: z.array(tagSchema),
  meta: pageMetaSchema,
});

export const securityResultStateSchema = z.enum(["QUEUED", "RUNNING", "COMPLETED", "FAILED", "STALE"]);
export const securityResultSchema = z.object({
  state: securityResultStateSchema,
  error_code: z.string().min(1).max(128).optional(),
  attempts: z.number().int().min(0),
  updated_at: timestampSchema,
  completed_at: timestampSchema.optional(),
  tool: z.object({
    name: z.literal("TRIVY"),
    scanner_version: z.string().min(1).max(128),
    database_schema_version: z.number().int().min(1),
    database_updated_at: timestampSchema,
    database_downloaded_at: timestampSchema,
  }).optional(),
  finding_count: z.number().int().min(0).optional(),
  severity_counts: z.record(z.string(), z.number().int().min(0)).optional(),
  format: z.literal("CYCLONEDX_JSON").optional(),
});

const securityScanResultSchema = securityResultSchema.superRefine((result, context) => {
  const hasCompletedEvidence = result.completed_at !== undefined && result.tool !== undefined &&
    result.finding_count !== undefined && result.severity_counts !== undefined;
  if ((result.state === "COMPLETED" || result.state === "STALE") && !hasCompletedEvidence) {
    context.addIssue({ code: "custom", message: "completed scan evidence is required" });
  }
  if (result.state === "FAILED" && result.error_code === undefined) {
    context.addIssue({ code: "custom", message: "failed scan error code is required" });
  }
});

const securitySBOMResultSchema = securityResultSchema.superRefine((result, context) => {
  if ((result.state === "COMPLETED" || result.state === "STALE") &&
      (result.completed_at === undefined || result.format === undefined)) {
    context.addIssue({ code: "custom", message: "completed SBOM evidence is required" });
  }
  if (result.state === "FAILED" && result.error_code === undefined) {
    context.addIssue({ code: "custom", message: "failed SBOM error code is required" });
  }
});

export const signatureEvidenceSchema = z.object({
  kind: z.enum(["SIGNATURE", "ATTESTATION"]),
  signature_digest: artifactDigestSchema,
  signer_type: z.enum(["PUBLIC_KEY", "KEYLESS", "UNKNOWN"]),
  key_fingerprint: artifactDigestSchema.optional(),
  oidc_issuer: z.url().startsWith("https://").optional(),
  subject: z.string().min(1).max(2048).optional(),
  cryptographic_state: z.enum(["VALID", "INVALID", "UNVERIFIED", "UNAVAILABLE"]),
  trust_state: z.enum(["TRUSTED", "UNTRUSTED", "NOT_EVALUATED"]),
  reason: z.string().min(1).max(128),
}).superRefine((evidence, context) => {
  const validTrust = evidence.cryptographic_state === "VALID"
    ? evidence.trust_state === "TRUSTED" || evidence.trust_state === "UNTRUSTED"
    : evidence.trust_state === "NOT_EVALUATED";
  const validSigner = evidence.signer_type === "PUBLIC_KEY"
    ? evidence.key_fingerprint !== undefined && evidence.oidc_issuer === undefined && evidence.subject === undefined
    : evidence.signer_type === "KEYLESS"
      ? evidence.key_fingerprint === undefined && evidence.oidc_issuer !== undefined && evidence.subject !== undefined
      : evidence.key_fingerprint === undefined && evidence.oidc_issuer === undefined && evidence.subject === undefined && evidence.cryptographic_state !== "VALID";
  if (!validTrust) context.addIssue({ code: "custom", message: "cryptographic and trust states conflict" });
  if (!validSigner) context.addIssue({ code: "custom", message: "signer evidence is incomplete or contradictory" });
});

export const signatureResultSchema = z.object({
  state: z.enum(["ABSENT", "QUEUED", "RUNNING", "COMPLETED", "FAILED", "STALE"]),
  error_code: z.string().min(1).max(128).optional(),
  attempts: z.number().int().min(0).optional(),
  updated_at: timestampSchema.optional(),
  policy_id: idSchema.optional(),
  policy_version: z.number().int().min(1).optional(),
  cosign_version: z.string().min(1).max(128).optional(),
  completed_at: timestampSchema.optional(),
  evidence: z.array(signatureEvidenceSchema),
}).superRefine((signature, context) => {
  if (signature.state === "ABSENT") {
    if (signature.evidence.length !== 0 || signature.policy_id !== undefined || signature.policy_version !== undefined ||
        signature.updated_at !== undefined || signature.completed_at !== undefined || signature.cosign_version !== undefined) {
      context.addIssue({ code: "custom", message: "absent verification cannot contain policy or evidence" });
    }
    return;
  }
  if (signature.policy_id === undefined || signature.policy_version === undefined || signature.updated_at === undefined) {
    context.addIssue({ code: "custom", message: "verification workflow identity is required" });
  }
  if ((signature.state === "COMPLETED" || signature.state === "STALE") &&
      (signature.cosign_version === undefined || signature.completed_at === undefined)) {
    context.addIssue({ code: "custom", message: "completed verification evidence is required" });
  }
  if (signature.state === "FAILED" && signature.error_code === undefined) {
    context.addIssue({ code: "custom", message: "failed verification error code is required" });
  }
  if ((signature.state === "QUEUED" || signature.state === "RUNNING" || signature.state === "FAILED") &&
      (signature.evidence.length !== 0 || signature.completed_at !== undefined || signature.cosign_version !== undefined)) {
    context.addIssue({ code: "custom", message: "unfinished verification cannot contain completed evidence" });
  }
});

export const artifactSecuritySchema = z.object({
  digest: artifactDigestSchema,
  scan: securityScanResultSchema,
  sbom: securitySBOMResultSchema,
  signature: signatureResultSchema,
});

export type HealthResponse = z.infer<typeof healthResponseSchema>;
export type ErrorEnvelope = z.infer<typeof errorEnvelopeSchema>;
export type FieldError = z.infer<typeof fieldErrorSchema>;
export type User = z.infer<typeof userSchema>;
export type LoginResponse = z.infer<typeof loginResponseSchema>;
export type OrganizationRole = z.infer<typeof organizationRoleSchema>;
export type Organization = z.infer<typeof organizationSchema>;
export type OrganizationList = z.infer<typeof organizationListSchema>;
export type Membership = z.infer<typeof membershipSchema>;
export type MembershipList = z.infer<typeof membershipListSchema>;
export type RepositoryVisibility = z.infer<typeof repositoryVisibilitySchema>;
export type Repository = z.infer<typeof repositorySchema>;
export type RepositoryCapabilities = z.infer<typeof repositoryCapabilitiesSchema>;
export type RepositoryDetail = z.infer<typeof repositoryDetailSchema>;
export type RepositoryList = z.infer<typeof repositoryListSchema>;
export type Artifact = z.infer<typeof artifactSchema>;
export type ArtifactList = z.infer<typeof artifactListSchema>;
export type ManifestDescriptor = z.infer<typeof manifestDescriptorSchema>;
export type Tag = z.infer<typeof tagSchema>;
export type TagList = z.infer<typeof tagListSchema>;
export type ArtifactSecurity = z.infer<typeof artifactSecuritySchema>;
export type SecurityResult = z.infer<typeof securityResultSchema>;
export type SignatureEvidence = z.infer<typeof signatureEvidenceSchema>;
export type SignatureResult = z.infer<typeof signatureResultSchema>;
