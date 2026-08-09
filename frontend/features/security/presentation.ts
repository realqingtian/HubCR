import type { MessageKey } from "@/lib/i18n/messages";
import type { SecurityResult, SignatureEvidence, SignatureResult } from "@/lib/api/schemas";

// Translate mirrors the (key, params?) => string signature exported by the i18n module. It
// is passed into the describe* helpers so this pure module never imports React hooks.
export type Translate = (key: MessageKey, params?: Record<string, string | number>) => string;

export type SecurityPresentation = Readonly<{
  title: string;
  detail: string;
  tone: "neutral" | "positive" | "warning" | "danger";
}>;

export function describeResult(t: Translate, label: "Scan" | "SBOM", result: SecurityResult): SecurityPresentation {
  switch (result.state) {
    case "QUEUED":
      return { title: t("security.result.queued", { label }), detail: t("security.result.queuedDetail"), tone: "neutral" };
    case "RUNNING":
      return { title: t("security.result.running", { label }), detail: t("security.result.runningDetail", { n: result.attempts }), tone: "neutral" };
    case "FAILED":
      return result.error_code?.includes("UNAVAILABLE")
        ? { title: t("security.result.unavailable", { label }), detail: t("security.result.unavailableDetail", { code: result.error_code }), tone: "warning" }
        : { title: t("security.result.failed", { label }), detail: result.error_code ?? t("security.result.failedDefaultDetail"), tone: "danger" };
    case "STALE":
      return { title: t("security.result.stale", { label }), detail: t("security.result.staleDetail"), tone: "warning" };
    case "COMPLETED":
      if (label === "Scan") {
        return result.finding_count === undefined
          ? { title: t("security.result.scanEvidenceUnavailable"), detail: t("security.result.scanEvidenceUnavailableDetail"), tone: "warning" }
          : { title: t("security.result.scanCompleted"), detail: t("security.result.scanCompletedDetail", { n: result.finding_count }), tone: "positive" };
      }
      return result.format === undefined
        ? { title: t("security.result.sbomEvidenceUnavailable"), detail: t("security.result.sbomEvidenceUnavailableDetail"), tone: "warning" }
        : { title: t("security.result.sbomCompleted"), detail: result.format, tone: "positive" };
  }
}

export function describeSignature(t: Translate, result: SignatureResult): SecurityPresentation {
  switch (result.state) {
    case "ABSENT":
      return { title: t("security.signature.notConfigured"), detail: "No trust-policy verification workflow exists for this Artifact.", tone: "neutral" };
    case "QUEUED":
      return { title: t("security.signature.queued"), detail: `Policy version ${result.policy_version} is waiting to run.`, tone: "neutral" };
    case "RUNNING":
      return { title: t("security.signature.running"), detail: `Policy version ${result.policy_version} is being evaluated.`, tone: "neutral" };
    case "FAILED":
      return result.error_code?.includes("UNAVAILABLE")
        ? { title: t("security.signature.unavailable"), detail: t("security.signature.unavailableDetail", { code: result.error_code }), tone: "warning" }
        : { title: t("security.signature.failed"), detail: result.error_code ?? "The worker reported a verification failure.", tone: "danger" };
    case "STALE":
      return { title: t("security.signature.stale"), detail: `These results use historical policy version ${result.policy_version}.`, tone: "warning" };
    case "COMPLETED":
      return result.evidence.length === 0
        ? { title: t("security.signature.none"), detail: `Cosign ${result.cosign_version} found no signature or attestation material.`, tone: "neutral" }
        : { title: t("security.signature.completed"), detail: t("security.signature.completedDetail", { n: result.evidence.length }), tone: "positive" };
  }
}

export function describeEvidence(t: Translate, evidence: SignatureEvidence): SecurityPresentation {
  if (evidence.cryptographic_state === "INVALID") {
    return { title: t("security.evidence.invalid"), detail: evidence.reason, tone: "danger" };
  }
  if (evidence.cryptographic_state === "UNAVAILABLE") {
    return { title: t("security.evidence.unavailable"), detail: evidence.reason, tone: "warning" };
  }
  if (evidence.cryptographic_state === "UNVERIFIED") {
    return { title: t("security.evidence.unverified"), detail: evidence.reason, tone: "warning" };
  }
  if (evidence.trust_state === "TRUSTED") {
    return { title: t("security.evidence.trusted"), detail: evidence.reason, tone: "positive" };
  }
  return { title: t("security.evidence.validUntrusted"), detail: evidence.reason, tone: "warning" };
}
