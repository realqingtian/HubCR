import { describe, expect, it } from "vitest";
import { describeEvidence, describeResult, describeSignature } from "./presentation";
import { messages } from "../../lib/i18n/messages";

// Test translate function: looks up the EN catalog and performs light {name} interpolation
// so the existing English assertions on title strings keep passing.
function t(key: keyof typeof messages.en, params?: Record<string, string | number>): string {
  const template = messages.en[key] ?? key;
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_match, name: string) =>
    params[name] !== undefined ? String(params[name]) : `{${name}}`,
  );
}

const timestamp = "2026-08-09T12:00:00Z";
const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

describe("truthful security presentation", () => {
  it("keeps operational states distinct", () => {
    expect(describeResult(t, "Scan", { state: "QUEUED", attempts: 0, updated_at: timestamp }).title).toBe("Scan queued");
    expect(describeResult(t, "Scan", { state: "RUNNING", attempts: 1, updated_at: timestamp }).title).toBe("Scan running");
    expect(describeResult(t, "Scan", { state: "FAILED", error_code: "SCANNER_UNAVAILABLE", attempts: 3, updated_at: timestamp }).title).toBe("Scan unavailable");
    expect(describeResult(t, "Scan", { state: "FAILED", error_code: "INVALID_OUTPUT", attempts: 1, updated_at: timestamp }).title).toBe("Scan failed");
    expect(describeResult(t, "Scan", { state: "STALE", attempts: 1, updated_at: timestamp }).title).toBe("Scan stale");
  });

  it("distinguishes absent and unsigned verification", () => {
    expect(describeSignature(t, { state: "ABSENT", evidence: [] }).title).toBe("Verification not configured");
    expect(describeSignature(t, {
      state: "COMPLETED", attempts: 1, updated_at: timestamp,
      policy_id: "33333333-3333-4333-8333-333333333333", policy_version: 2,
      cosign_version: "v3.0.6", completed_at: timestamp, evidence: [],
    }).title).toBe("No signatures discovered");
  });

  it("keeps every verification workflow state distinct", () => {
    const workflow = {
      attempts: 1, updated_at: timestamp,
      policy_id: "33333333-3333-4333-8333-333333333333", policy_version: 2,
      evidence: [],
    };
    expect(describeSignature(t, { ...workflow, state: "QUEUED" }).title).toBe("Verification queued");
    expect(describeSignature(t, { ...workflow, state: "RUNNING" }).title).toBe("Verification running");
    expect(describeSignature(t, { ...workflow, state: "FAILED", error_code: "COSIGN_UNAVAILABLE" }).title).toBe("Verification unavailable");
    expect(describeSignature(t, { ...workflow, state: "FAILED", error_code: "INVALID_OUTPUT" }).title).toBe("Verification failed");
    expect(describeSignature(t, {
      ...workflow, state: "STALE", cosign_version: "v3.0.6", completed_at: timestamp,
    }).title).toBe("Verification stale");
  });

  it("renders backend validity and trust without treating presence as trust", () => {
    const base = { kind: "SIGNATURE" as const, signature_digest: digest, reason: "evidence" };
    expect(describeEvidence(t, { ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "VALID", trust_state: "TRUSTED" }).title).toBe("Trusted");
    expect(describeEvidence(t, { ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "VALID", trust_state: "UNTRUSTED" }).title).toBe("Valid, untrusted");
    expect(describeEvidence(t, { ...base, signer_type: "UNKNOWN", cryptographic_state: "UNVERIFIED", trust_state: "NOT_EVALUATED" }).title).toBe("Unverified");
    expect(describeEvidence(t, { ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "INVALID", trust_state: "NOT_EVALUATED" }).title).toBe("Invalid");
    expect(describeEvidence(t, { ...base, signer_type: "UNKNOWN", cryptographic_state: "UNAVAILABLE", trust_state: "NOT_EVALUATED" }).title).toBe("Unavailable");
  });
});
