"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { createRepository, listRepositories, updateRepository, type RepositoryVisibility } from "@/lib/api/client";
import { PanelMessage, useFriendlyError } from "@/features/shared/feedback";
import { useLocale, useT } from "@/lib/i18n";

export function RepositoryPanel({ namespace, title }: Readonly<{ namespace: string; title: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const friendlyError = useFriendlyError();
  const queryClient = useQueryClient();
  const queryKey = ["repositories", namespace] as const;
  const repositories = useQuery({ queryKey, queryFn: () => listRepositories(namespace, { limit: 100 }) });
  const createMutation = useMutation({
    mutationFn: (input: { name: string; visibility: RepositoryVisibility; description: string }) => createRepository(namespace, input.name, input.visibility, input.description),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey }); },
  });
  const visibilityMutation = useMutation({
    mutationFn: ({ name, visibility }: { name: string; visibility: RepositoryVisibility }) => updateRepository(namespace, name, { visibility }),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey }); },
  });

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    createMutation.mutate({
      name: String(data.get("name") ?? ""),
      visibility: String(data.get("visibility") ?? "PRIVATE") as RepositoryVisibility,
      description: String(data.get("description") ?? ""),
    }, { onSuccess: () => form.reset() });
  }

  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-mono text-xs text-sky-700">hubcr.io/{namespace}</p>
          <h3 className="mt-1 text-lg font-semibold text-slate-950">{title}</h3>
        </div>
        <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-600">{t("repository.metadataBadge")}</span>
      </div>

      <form className="mt-5 grid gap-3 rounded-xl bg-slate-50 p-4 sm:grid-cols-2" onSubmit={submit}>
        <div>
          <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-repository-name`}>{t("repository.field.name")}</label>
          <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-repository-name`} name="name" pattern="[A-Za-z0-9]+([._-][A-Za-z0-9]+)*" maxLength={64} required />
        </div>
        <div>
          <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-repository-visibility`}>{t("repository.field.visibility")}</label>
          <select className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" defaultValue="PRIVATE" id={`${namespace}-repository-visibility`} name="visibility">
            <option value="PRIVATE">{t("repository.visibility.private")}</option>
            <option value="PUBLIC">{t("repository.visibility.public")}</option>
          </select>
        </div>
        <div className="sm:col-span-2">
          <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-repository-description`}>{t("repository.field.description")}</label>
          <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-repository-description`} name="description" maxLength={1024} />
        </div>
        {createMutation.isError ? <p className="text-sm text-rose-700 sm:col-span-2" role="alert">{friendlyError(createMutation.error)}</p> : null}
        <button className="rounded-lg bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200 disabled:opacity-60 sm:col-span-2" disabled={createMutation.isPending} type="submit">
          {createMutation.isPending ? t("repository.create.pending") : t("repository.create.default")}
        </button>
      </form>

      <div className="mt-5 space-y-3">
        {repositories.isPending ? <PanelMessage title={t("repository.loading")} detail={t("repository.loadingDetail")} /> : null}
        {repositories.isError ? <PanelMessage title={t("repository.unavailable")} detail={friendlyError(repositories.error)} tone="error" /> : null}
        {repositories.data?.items.length === 0 ? <PanelMessage title={t("repository.empty")} detail={t("repository.emptyDetail")} /> : null}
        {repositories.data?.items.map((repository) => {
          const nextVisibility = repository.visibility === "PRIVATE" ? "PUBLIC" : "PRIVATE";
          const changing = visibilityMutation.isPending && visibilityMutation.variables?.name === repository.name;
          const visibilityWord = nextVisibility === "PUBLIC" ? t("repository.visibility.public") : t("repository.visibility.private");
          return (
            <article className="flex flex-col gap-4 rounded-xl border border-slate-200 p-4 sm:flex-row sm:items-center sm:justify-between" key={repository.id}>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h4 className="truncate font-mono text-sm font-semibold text-slate-950">
                    <Link className="rounded hover:text-sky-700 focus:outline-none focus:ring-4 focus:ring-sky-100" href={`/namespaces/${namespace}/repositories/${repository.name}`}>{namespace}/{repository.name}</Link>
                  </h4>
                  <span className={repository.visibility === "PUBLIC" ? "rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700" : "rounded-full bg-amber-50 px-2.5 py-1 text-xs font-semibold text-amber-800"}>{repository.visibility}</span>
                </div>
                <p className="mt-1 text-sm text-slate-600">{repository.description || t("org.noDescription")}</p>
                <p className="mt-2 text-xs text-slate-400">{t("repository.visibilityUpdated", { date: new Date(repository.visibility_updated_at).toLocaleString(locale) })}</p>
              </div>
              <div className="flex shrink-0 flex-wrap gap-2">
                <Link className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" href={`/namespaces/${namespace}/repositories/${repository.name}`}>{t("repository.view")}</Link>
                <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100 disabled:opacity-60" disabled={visibilityMutation.isPending} onClick={() => visibilityMutation.mutate({ name: repository.name, visibility: nextVisibility })} type="button">
                  {changing ? t("repository.updating") : t("repository.makeVisibility", { visibility: visibilityWord })}
                </button>
              </div>
            </article>
          );
        })}
        {visibilityMutation.isError ? <PanelMessage title={t("repository.visibilityUnchanged")} detail={friendlyError(visibilityMutation.error)} tone="error" /> : null}
        {repositories.data?.meta.next_cursor ? <p className="text-xs text-slate-500">{t("repository.paginationNote")}</p> : null}
      </div>
    </section>
  );
}
