"use client";

import { useQuery } from "@tanstack/react-query";
import { PanelMessage, useFriendlyError } from "@/features/shared/feedback";
import { getArtifactSecurity, type SecurityResult, type SignatureEvidence } from "@/lib/api/client";
import { useLocale, useT } from "@/lib/i18n";
import { describeEvidence, describeResult, describeSignature, type SecurityPresentation } from "./presentation";

export function ArtifactSecurityPanel({ digest, namespace, repository }: Readonly<{
  digest: string;
  namespace: string;
  repository: string;
}>) {
  const t = useT();
  const friendlyError = useFriendlyError();
  const security = useQuery({
    queryKey: ["repositories", namespace, repository, "artifacts", digest, "security"],
    queryFn: () => getArtifactSecurity(namespace, repository, digest),
    retry: false,
  });

  if (security.isPending) {
    return <PanelMessage title={t("security.loading")} detail={t("security.loadingDetail")} />;
  }
  if (security.isError) {
    return (
      <section aria-labelledby="artifact-security" className="space-y-3 rounded-2xl border border-rose-200 bg-rose-50 p-5 sm:p-6">
        <h2 className="text-lg font-semibold text-rose-950" id="artifact-security">{t("security.unavailable")}</h2>
        <p className="text-sm leading-6 text-rose-800">{friendlyError(security.error)}</p>
        <button className="rounded-lg border border-rose-300 bg-white px-3 py-2 text-sm font-semibold text-rose-900 hover:bg-rose-100 focus:outline-none focus:ring-4 focus:ring-rose-200" onClick={() => void security.refetch()} type="button">{t("shell.retry")}</button>
      </section>
    );
  }

  const detail = security.data;
  return (
    <section aria-labelledby="artifact-security" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-sky-700">{t("security.evidence")}</p>
          <h2 className="mt-2 text-xl font-semibold text-slate-950" id="artifact-security">{t("security.title")}</h2>
        </div>
        <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={() => void security.refetch()} type="button">{t("security.refresh")}</button>
      </div>

      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <ResultCard presentation={describeResult(t, "Scan", detail.scan)} result={detail.scan} />
        <ResultCard presentation={describeResult(t, "SBOM", detail.sbom)} result={detail.sbom} />
      </div>

      <div className="mt-4 rounded-xl border border-slate-200 p-4">
        <PresentationHeader presentation={describeSignature(t, detail.signature)} />
        {detail.signature.policy_version ? <p className="mt-2 text-xs text-slate-500">Policy version {detail.signature.policy_version}{detail.signature.cosign_version ? ` · Cosign ${detail.signature.cosign_version}` : ""}</p> : null}
        {detail.signature.evidence.length > 0 ? (
          <div className="mt-4 grid gap-3 lg:grid-cols-2">
            {detail.signature.evidence.map((evidence) => <EvidenceCard evidence={evidence} key={`${evidence.kind}:${evidence.signature_digest}`} />)}
          </div>
        ) : null}
      </div>
    </section>
  );
}

function ResultCard({ presentation, result }: Readonly<{ presentation: SecurityPresentation; result: SecurityResult }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <article className="rounded-xl bg-slate-50 p-4">
      <PresentationHeader presentation={presentation} />
      <p className="mt-2 text-xs text-slate-500">{t("security.updated", { date: new Date(result.updated_at).toLocaleString(locale) })}</p>
    </article>
  );
}

function EvidenceCard({ evidence }: Readonly<{ evidence: SignatureEvidence }>) {
  const t = useT();
  const presentation = describeEvidence(t, evidence);
  const signer = evidence.signer_type === "PUBLIC_KEY"
    ? evidence.key_fingerprint
    : evidence.signer_type === "KEYLESS"
      ? `${evidence.oidc_issuer} · ${evidence.subject}`
      : t("security.signerUnavailable");
  return (
    <article className="rounded-xl bg-slate-50 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs font-semibold text-slate-600">{evidence.kind}</span>
        <StatusBadge presentation={presentation} />
      </div>
      <p className="mt-3 break-all font-mono text-xs leading-5 text-slate-700">{evidence.signature_digest}</p>
      <p className="mt-2 break-all text-xs leading-5 text-slate-500">{signer}</p>
      <p className="mt-2 text-xs leading-5 text-slate-600">{presentation.detail}</p>
    </article>
  );
}

function PresentationHeader({ presentation }: Readonly<{ presentation: SecurityPresentation }>) {
  return (
    <div>
      <StatusBadge presentation={presentation} />
      <p className="mt-2 text-sm leading-6 text-slate-600">{presentation.detail}</p>
    </div>
  );
}

function StatusBadge({ presentation }: Readonly<{ presentation: SecurityPresentation }>) {
  const styles = {
    neutral: "bg-slate-200 text-slate-800",
    positive: "bg-emerald-100 text-emerald-800",
    warning: "bg-amber-100 text-amber-900",
    danger: "bg-rose-100 text-rose-800",
  } as const;
  return <span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-semibold ${styles[presentation.tone]}`}>{presentation.title}</span>;
}
