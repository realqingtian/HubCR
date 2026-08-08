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
