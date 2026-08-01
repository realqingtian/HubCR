import { z } from "zod";

const idSchema = z.string().uuid();
const timestampSchema = z.iso.datetime({ offset: true });
const namespaceNameSchema = z
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

export const repositoryListSchema = z.object({
  items: z.array(repositorySchema),
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
export type RepositoryList = z.infer<typeof repositoryListSchema>;
