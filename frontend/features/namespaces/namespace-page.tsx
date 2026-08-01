"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { friendlyError, PanelMessage } from "@/features/shared/feedback";
import { listRepositories } from "@/lib/api/client";

export function NamespacePage({ namespace }: Readonly<{ namespace: string }>) {
  const repositories = useQuery({
    queryKey: ["repositories", namespace],
    queryFn: () => listRepositories(namespace, { limit: 100 }),
    retry: false,
  });

  return (
    <div>
      <nav aria-label="Breadcrumb" className="text-sm text-slate-500">
        <ol className="flex flex-wrap items-center gap-2">
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href="/">Overview</Link></li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-slate-800">{namespace}</li>
        </ol>
      </nav>

      <section className="mt-5 rounded-3xl bg-slate-950 px-6 py-7 text-white shadow-sm sm:px-8">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-sky-300">Namespace</p>
        <h1 className="mt-3 break-all font-mono text-2xl font-semibold tracking-tight sm:text-3xl">hubcr.io/{namespace}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">Repositories shown here come from the authenticated control-plane discovery policy.</p>
      </section>

      <section aria-labelledby="namespace-repositories" className="mt-7">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">Repository discovery</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950" id="namespace-repositories">Repositories</h2>
          </div>
          <Link className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" href="/">Manage repositories</Link>
        </div>

        <div className="mt-5 space-y-3">
          {repositories.isPending ? <PanelMessage title="Loading repositories" detail="Reading the authorized namespace view." /> : null}
          {repositories.isError ? (
            <div className="space-y-3">
              <PanelMessage title="Namespace unavailable" detail={friendlyError(repositories.error)} tone="error" />
              <button className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={() => void repositories.refetch()} type="button">Try again</button>
            </div>
          ) : null}
          {repositories.data?.items.length === 0 ? <PanelMessage title="No repositories visible" detail="This namespace has no repositories your current session can discover." /> : null}
          {repositories.data?.items.map((repository) => (
            <article className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" key={repository.id}>
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="truncate font-mono text-base font-semibold text-slate-950">{repository.namespace}/{repository.name}</h3>
                    <VisibilityBadge visibility={repository.visibility} />
                  </div>
                  <p className="mt-2 text-sm leading-6 text-slate-600">{repository.description || "No description"}</p>
                </div>
                <Link className="shrink-0 rounded-lg bg-slate-950 px-4 py-2.5 text-center text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200" href={`/namespaces/${repository.namespace}/repositories/${repository.name}`}>View repository</Link>
              </div>
            </article>
          ))}
          {repositories.data?.meta.next_cursor ? <p className="text-xs text-slate-500">Showing the first 100 authorized repositories.</p> : null}
        </div>
      </section>
    </div>
  );
}

function VisibilityBadge({ visibility }: Readonly<{ visibility: "PUBLIC" | "PRIVATE" }>) {
  return <span className={visibility === "PUBLIC" ? "rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700" : "rounded-full bg-amber-50 px-2.5 py-1 text-xs font-semibold text-amber-800"}>{visibility}</span>;
}
