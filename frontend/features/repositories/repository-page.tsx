"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { ArtifactExplorer } from "@/features/artifacts/artifact-explorer";
import { PanelMessage, useFriendlyError } from "@/features/shared/feedback";
import { RepositoryQuickStart } from "@/features/repositories/repository-quick-start";
import { APIError, getRepository } from "@/lib/api/client";
import { useLocale, useT } from "@/lib/i18n";

export function RepositoryPage({ namespace, repository }: Readonly<{ namespace: string; repository: string }>) {
  const t = useT();
  const detail = useQuery({
    queryKey: ["repositories", namespace, repository],
    queryFn: () => getRepository(namespace, repository),
    retry: false,
  });

  return (
    <div>
      <nav aria-label="Breadcrumb" className="text-sm text-slate-500">
        <ol className="flex flex-wrap items-center gap-2">
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href="/">{t("namespace.breadcrumb.overview")}</Link></li>
          <li aria-hidden="true">/</li>
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href={`/namespaces/${namespace}`}>{namespace}</Link></li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-slate-800">{repository}</li>
        </ol>
      </nav>

      <div className="mt-5">
        {detail.isPending ? <PanelMessage title={t("repositoryPage.loading")} detail={t("repositoryPage.loadingDetail")} /> : null}
        {detail.isError ? <RepositoryFailure error={detail.error} retry={() => void detail.refetch()} /> : null}
        {detail.data ? <RepositoryDetail repository={detail.data} /> : null}
      </div>
    </div>
  );
}

function RepositoryFailure({ error, retry }: Readonly<{ error: unknown; retry: () => void }>) {
  const t = useT();
  const friendlyError = useFriendlyError();
  const missing = error instanceof APIError && error.code === "not_found";
  return (
    <section className="space-y-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
      <PanelMessage
        title={missing ? t("repositoryPage.notFound") : t("repositoryPage.unavailable")}
        detail={missing ? t("repositoryPage.notFoundDetail") : friendlyError(error)}
        tone="error"
      />
      <div className="flex flex-wrap gap-3">
        {!missing ? <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={retry} type="button">{t("shell.retry")}</button> : null}
        <Link className="rounded-lg bg-slate-950 px-3 py-2 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200" href="/">{t("repositoryPage.back")}</Link>
      </div>
    </section>
  );
}

function RepositoryDetail({ repository }: Readonly<{ repository: Awaited<ReturnType<typeof getRepository>> }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="space-y-6">
      <section className="rounded-3xl bg-slate-950 px-6 py-7 text-white shadow-sm sm:px-8">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-sky-300">{t("repositoryPage.eyebrow")}</p>
            <h1 className="mt-3 break-all font-mono text-2xl font-semibold tracking-tight sm:text-3xl">{repository.namespace}/{repository.name}</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">{repository.description || t("org.noDescription")}</p>
          </div>
          <span className={repository.visibility === "PUBLIC" ? "self-start rounded-full bg-emerald-400/15 px-3 py-1.5 text-xs font-semibold text-emerald-200" : "self-start rounded-full bg-amber-400/15 px-3 py-1.5 text-xs font-semibold text-amber-200"}>{repository.visibility}</span>
        </div>
      </section>

      <section aria-labelledby="repository-metadata" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <h2 className="text-lg font-semibold text-slate-950" id="repository-metadata">{t("repositoryPage.metadata")}</h2>
        <dl className="mt-5 grid gap-4 text-sm sm:grid-cols-2">
          <Metadata label={t("repositoryPage.fact.registryPath")} value={`hubcr.io/${repository.namespace}/${repository.name}`} mono />
          <Metadata label={t("repositoryPage.fact.visibility")} value={repository.visibility} />
          <Metadata label={t("repositoryPage.fact.created")} value={new Date(repository.created_at).toLocaleString(locale)} />
          <Metadata label={t("repositoryPage.fact.visibilityUpdated")} value={new Date(repository.visibility_updated_at).toLocaleString(locale)} />
        </dl>
      </section>

      <RepositoryQuickStart
        capabilities={repository.capabilities}
        namespace={repository.namespace}
        repository={repository.name}
        visibility={repository.visibility}
      />

      <ArtifactExplorer namespace={repository.namespace} repository={repository.name} />
    </div>
  );
}

function Metadata({ label, mono = false, value }: Readonly<{ label: string; mono?: boolean; value: string }>) {
  return (
    <div className="rounded-xl bg-slate-50 p-4">
      <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className={`mt-2 break-all text-slate-900 ${mono ? "font-mono" : ""}`}>{value}</dd>
    </div>
  );
}
