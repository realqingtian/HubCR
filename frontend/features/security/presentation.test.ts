import { describe, expect, it } from "vitest";
import { describeEvidence, describeResult, describeSignature } from "./presentation";

const timestamp = "2026-08-09T12:00:00Z";
const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

describe("truthful security presentation", () => {
  it("keeps operational states distinct", () => {
    expect(describeResult("Scan", { state: "QUEUED", attempts: 0, updated_at: timestamp }).title).toBe("Scan queued");
    expect(describeResult("Scan", { state: "RUNNING", attempts: 1, updated_at: timestamp }).title).toBe("Scan running");
    expect(describeResult("Scan", { state: "FAILED", error_code: "SCANNER_UNAVAILABLE", attempts: 3, updated_at: timestamp }).title).toBe("Scan unavailable");
    expect(describeResult("Scan", { state: "FAILED", error_code: "INVALID_OUTPUT", attempts: 1, updated_at: timestamp }).title).toBe("Scan failed");
    expect(describeResult("Scan", { state: "STALE", attempts: 1, updated_at: timestamp }).title).toBe("Scan stale");
  });

  it("distinguishes absent and unsigned verification", () => {
    expect(describeSignature({ state: "ABSENT", evidence: [] }).title).toBe("Verification not configured");
    expect(describeSignature({
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
    expect(describeSignature({ ...workflow, state: "QUEUED" }).title).toBe("Verification queued");
    expect(describeSignature({ ...workflow, state: "RUNNING" }).title).toBe("Verification running");
    expect(describeSignature({ ...workflow, state: "FAILED", error_code: "COSIGN_UNAVAILABLE" }).title).toBe("Verification unavailable");
    expect(describeSignature({ ...workflow, state: "FAILED", error_code: "INVALID_OUTPUT" }).title).toBe("Verification failed");
    expect(describeSignature({
      ...workflow, state: "STALE", cosign_version: "v3.0.6", completed_at: timestamp,
    }).title).toBe("Verification stale");
  });

  it("renders backend validity and trust without treating presence as trust", () => {
    const base = { kind: "SIGNATURE" as const, signature_digest: digest, reason: "evidence" };
    expect(describeEvidence({ ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "VALID", trust_state: "TRUSTED" }).title).toBe("Trusted");
    expect(describeEvidence({ ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "VALID", trust_state: "UNTRUSTED" }).title).toBe("Valid, untrusted");
    expect(describeEvidence({ ...base, signer_type: "UNKNOWN", cryptographic_state: "UNVERIFIED", trust_state: "NOT_EVALUATED" }).title).toBe("Unverified");
    expect(describeEvidence({ ...base, signer_type: "PUBLIC_KEY", key_fingerprint: digest, cryptographic_state: "INVALID", trust_state: "NOT_EVALUATED" }).title).toBe("Invalid");
    expect(describeEvidence({ ...base, signer_type: "UNKNOWN", cryptographic_state: "UNAVAILABLE", trust_state: "NOT_EVALUATED" }).title).toBe("Unavailable");
  });
});
