"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { PanelMessage, useFriendlyError } from "@/features/shared/feedback";
import { APIError, listArtifacts, listTags, type Artifact, type Tag } from "@/lib/api/client";
import { useT } from "@/lib/i18n";
import { type MessageKey } from "@/lib/i18n/messages";

export function ArtifactExplorer({ namespace, repository }: Readonly<{ namespace: string; repository: string }>) {
  const t = useT();
  const tags = useQuery({
    queryKey: ["repositories", namespace, repository, "tags"],
    queryFn: () => listTags(namespace, repository, { limit: 100 }),
    retry: false,
  });
  const artifacts = useQuery({
    queryKey: ["repositories", namespace, repository, "artifacts"],
    queryFn: () => listArtifacts(namespace, repository, { limit: 100 }),
    retry: false,
  });

  return (
    <section aria-labelledby="artifact-explorer" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">{t("artifacts.eyebrow")}</p>
        <h2 className="mt-2 text-xl font-semibold text-slate-950" id="artifact-explorer">{t("artifacts.title")}</h2>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">{t("artifacts.detail")}</p>
      </div>

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <ArtifactColumn
          error={tags.error}
          isError={tags.isError}
          isPending={tags.isPending}
          onRetry={() => void tags.refetch()}
          title={t("artifacts.tags")}
        >
          {tags.data?.items.length === 0 ? <PanelMessage title={t("artifacts.tagsEmpty")} detail={t("artifacts.tagsEmptyDetail")} /> : null}
          {tags.data?.items.map((tag) => <TagCard key={tag.name} namespace={namespace} repository={repository} tag={tag} />)}
          {tags.data?.meta.next_cursor ? <p className="text-xs text-slate-500">{t("artifacts.tagsPagination")}</p> : null}
        </ArtifactColumn>

        <ArtifactColumn
          error={artifacts.error}
          isError={artifacts.isError}
          isPending={artifacts.isPending}
          onRetry={() => void artifacts.refetch()}
          title={t("artifacts.digests")}
        >
          {artifacts.data?.items.length === 0 ? <PanelMessage title={t("artifacts.digestsEmpty")} detail={t("artifacts.digestsEmptyDetail")} /> : null}
          {artifacts.data?.items.map((artifact) => <ArtifactCard artifact={artifact} key={artifact.digest} namespace={namespace} repository={repository} />)}
          {artifacts.data?.meta.next_cursor ? <p className="text-xs text-slate-500">{t("artifacts.digestsPagination")}</p> : null}
        </ArtifactColumn>
      </div>
    </section>
  );
}

function ArtifactColumn({ children, error, isError, isPending, onRetry, title }: Readonly<{
  children: React.ReactNode;
  error: unknown;
  isError: boolean;
  isPending: boolean;
  onRetry: () => void;
  title: string;
}>) {
  const t = useT();
  const friendlyError = useFriendlyError();
  return (
    <div className="min-w-0">
      <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      <div className="mt-3 space-y-3">
        {isPending ? <PanelMessage title={t("artifacts.loading", { title: title.toLowerCase() })} detail={t("artifacts.loadingDetail")} /> : null}
        {isError ? (
          <div className="space-y-3">
            <PanelMessage title={columnErrorTitle(t, title, error)} detail={friendlyError(error)} tone="error" />
            <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={onRetry} type="button">{t("shell.retry")}</button>
          </div>
        ) : null}
        {!isPending && !isError ? children : null}
      </div>
    </div>
  );
}

type Translate = (key: MessageKey, params?: Record<string, string | number>) => string;

function columnErrorTitle(t: Translate, title: string, error: unknown): string {
  if (error instanceof APIError) {
    return error.code === "forbidden" ? t("artifacts.forbidden", { title }) : t("artifacts.requestFailed", { title });
  }
  return t("artifacts.unavailable", { title });
}

function TagCard({ namespace, repository, tag }: Readonly<{ namespace: string; repository: string; tag: Tag }>) {
  const t = useT();
  return (
    <article className="rounded-xl border border-slate-200 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="font-mono text-sm font-semibold text-slate-950">{tag.name}</h4>
        <span className="rounded-full bg-violet-50 px-2.5 py-1 text-xs font-semibold text-violet-700">{t("artifacts.tagBadge")}</span>
      </div>
      <p className="mt-3 break-all font-mono text-xs leading-5 text-slate-600">{tag.digest}</p>
      <Link className="mt-4 inline-flex rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" href={artifactPath(namespace, repository, tag.digest)}>{t("artifacts.viewArtifact")}</Link>
    </article>
  );
}

function ArtifactCard({ artifact, namespace, repository }: Readonly<{ artifact: Artifact; namespace: string; repository: string }>) {
  const t = useT();
  return (
    <article className="rounded-xl border border-slate-200 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="rounded-full bg-sky-50 px-2.5 py-1 text-xs font-semibold text-sky-700">{artifact.kind}</span>
        <span className="text-xs text-slate-500">{formatBytes(t, artifact.size_bytes)}</span>
      </div>
      <p className="mt-3 break-all font-mono text-xs leading-5 text-slate-700">{artifact.digest}</p>
      <Link className="mt-4 inline-flex rounded-lg bg-slate-950 px-3 py-2 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200" href={artifactPath(namespace, repository, artifact.digest)}>{t("artifacts.viewDetails")}</Link>
    </article>
  );
}

function artifactPath(namespace: string, repository: string, digest: string): string {
  return `/namespaces/${namespace}/repositories/${repository}/artifacts/${encodeURIComponent(digest)}`;
}

function formatBytes(t: Translate, value: number | undefined): string {
  if (value === undefined) return t("artifacts.sizeUnavailable");
  if (value < 1024) return `${value} B`;
  return `${(value / 1024).toFixed(1)} KiB`;
}
