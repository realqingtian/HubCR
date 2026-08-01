"use client";

import Link from "next/link";
import { useAuthenticatedUser } from "@/features/auth/authenticated-shell";
import { OrganizationWorkspace } from "./organization-workspace";
import { RepositoryPanel } from "./repository-panel";

export function ControlPlaneWorkspace() {
  const user = useAuthenticatedUser();
  return (
    <div>
      <section className="rounded-3xl bg-slate-950 px-6 py-7 text-white shadow-sm sm:px-8">
        <div className="flex flex-col justify-between gap-6 sm:flex-row sm:items-center">
          <div>
            <p className="text-sm text-slate-400">Authenticated workspace</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-tight">Control-plane workspace</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">Manage business metadata and policy. Image bytes continue to flow through CNCF Distribution.</p>
          </div>
          <Link className="self-start rounded-lg border border-white/20 px-4 py-2.5 text-sm font-semibold text-white hover:bg-white/10 focus:outline-none focus:ring-4 focus:ring-white/20" href={`/namespaces/${user.personal_namespace}`}>Open namespace</Link>
        </div>
      </section>

      <section className="mt-8 grid gap-5 lg:grid-cols-[0.72fr_1.28fr]">
        <article className="rounded-2xl border border-sky-200 bg-sky-50 p-5 sm:p-6">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">Personal namespace</p>
          <p className="mt-4 break-all font-mono text-lg font-semibold text-slate-950">hubcr.io/{user.personal_namespace}</p>
          <p className="mt-3 text-sm leading-6 text-slate-600">This normalized namespace is returned explicitly by the authenticated user API.</p>
          <Link className="mt-5 inline-flex rounded-lg border border-sky-300 bg-white px-3 py-2 text-sm font-semibold text-sky-800 hover:bg-sky-100 focus:outline-none focus:ring-4 focus:ring-sky-100" href={`/namespaces/${user.personal_namespace}`}>View namespace</Link>
        </article>
        <RepositoryPanel namespace={user.personal_namespace} title="Personal repositories" />
      </section>

      <OrganizationWorkspace />
    </div>
  );
}
