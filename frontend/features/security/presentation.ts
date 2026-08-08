import type { SecurityResult, SignatureEvidence, SignatureResult } from "@/lib/api/schemas";

export type SecurityPresentation = Readonly<{
  title: string;
  detail: string;
  tone: "neutral" | "positive" | "warning" | "danger";
}>;

export function describeResult(label: "Scan" | "SBOM", result: SecurityResult): SecurityPresentation {
  switch (result.state) {
    case "QUEUED":
      return { title: `${label} queued`, detail: "The durable worker job is waiting to run.", tone: "neutral" };
    case "RUNNING":
      return { title: `${label} running`, detail: `Attempt ${result.attempts} is in progress.`, tone: "neutral" };
    case "FAILED":
      return result.error_code?.includes("UNAVAILABLE")
        ? { title: `${label} unavailable`, detail: `The dependency is unavailable (${result.error_code}).`, tone: "warning" }
        : { title: `${label} failed`, detail: result.error_code ?? "The worker reported a failure.", tone: "danger" };
    case "STALE":
      return { title: `${label} stale`, detail: "Stored evidence was produced by an older tool or database version.", tone: "warning" };
    case "COMPLETED":
      if (label === "Scan") {
        return result.finding_count === undefined
          ? { title: "Scan evidence unavailable", detail: "The completed result did not include a finding count.", tone: "warning" }
          : { title: "Scan completed", detail: `${result.finding_count} vulnerability findings recorded.`, tone: "positive" };
      }
      return result.format === undefined
        ? { title: "SBOM evidence unavailable", detail: "The completed result did not include an SBOM format.", tone: "warning" }
        : { title: "SBOM completed", detail: result.format, tone: "positive" };
  }
}

export function describeSignature(result: SignatureResult): SecurityPresentation {
  switch (result.state) {
    case "ABSENT":
      return { title: "Verification not configured", detail: "No trust-policy verification workflow exists for this Artifact.", tone: "neutral" };
    case "QUEUED":
      return { title: "Verification queued", detail: `Policy version ${result.policy_version} is waiting to run.`, tone: "neutral" };
    case "RUNNING":
      return { title: "Verification running", detail: `Policy version ${result.policy_version} is being evaluated.`, tone: "neutral" };
    case "FAILED":
      return result.error_code?.includes("UNAVAILABLE")
        ? { title: "Verification unavailable", detail: `Cosign or Registry access is unavailable (${result.error_code}).`, tone: "warning" }
        : { title: "Verification failed", detail: result.error_code ?? "The worker reported a verification failure.", tone: "danger" };
    case "STALE":
      return { title: "Verification stale", detail: `These results use historical policy version ${result.policy_version}.`, tone: "warning" };
    case "COMPLETED":
      return result.evidence.length === 0
        ? { title: "No signatures discovered", detail: `Cosign ${result.cosign_version} found no signature or attestation material.`, tone: "neutral" }
        : { title: "Verification completed", detail: `${result.evidence.length} signature or attestation records were evaluated.`, tone: "positive" };
  }
}

export function describeEvidence(evidence: SignatureEvidence): SecurityPresentation {
  if (evidence.cryptographic_state === "INVALID") {
    return { title: "Invalid", detail: evidence.reason, tone: "danger" };
  }
  if (evidence.cryptographic_state === "UNAVAILABLE") {
    return { title: "Unavailable", detail: evidence.reason, tone: "warning" };
  }
  if (evidence.cryptographic_state === "UNVERIFIED") {
    return { title: "Unverified", detail: evidence.reason, tone: "warning" };
  }
  if (evidence.trust_state === "TRUSTED") {
    return { title: "Trusted", detail: evidence.reason, tone: "positive" };
  }
  return { title: "Valid, untrusted", detail: evidence.reason, tone: "warning" };
}
