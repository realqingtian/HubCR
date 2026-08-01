"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIError, getCurrentUser, logout, type LoginResponse } from "@/lib/api/client";
import { friendlyError, PanelMessage } from "./feedback";
import { LoginPanel } from "./login-panel";
import { OrganizationWorkspace } from "./organization-workspace";
import { RepositoryPanel } from "./repository-panel";

export function ControlPlaneWorkspace() {
  const queryClient = useQueryClient();
  const currentUser = useQuery({
    queryKey: ["auth", "me"],
    queryFn: ({ signal }) => getCurrentUser(signal),
    retry: false,
  });
  const logoutMutation = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.clear();
      void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });

  function loggedIn(result: LoginResponse) {
    queryClient.setQueryData(["auth", "me"], result.user);
  }

  if (currentUser.isPending) {
    return <PanelMessage title="Checking your session" detail="Connecting to the HubCR control plane." />;
  }
  if (currentUser.isError) {
    if (currentUser.error instanceof APIError && currentUser.error.code === "authentication_failed") {
      return <LoginPanel onSuccess={loggedIn} />;
    }
    return (
      <div className="space-y-4">
        <PanelMessage title="Control plane unavailable" detail={friendlyError(currentUser.error)} tone="error" />
        <button className="rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={() => void currentUser.refetch()} type="button">Try again</button>
      </div>
    );
  }

  const user = currentUser.data;
  return (
    <div>
      <section className="rounded-3xl bg-slate-950 px-6 py-7 text-white shadow-sm sm:px-8">
        <div className="flex flex-col justify-between gap-6 sm:flex-row sm:items-center">
          <div>
            <p className="text-sm text-slate-400">Signed in as <span className="font-medium text-white">{user.username}</span></p>
            <h1 className="mt-2 text-3xl font-semibold tracking-tight">Control-plane workspace</h1>
            <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300">Manage business metadata and policy. Image bytes continue to flow through CNCF Distribution.</p>
          </div>
          <button className="self-start rounded-lg border border-white/20 px-4 py-2.5 text-sm font-semibold text-white hover:bg-white/10 focus:outline-none focus:ring-4 focus:ring-white/20 disabled:opacity-60" disabled={logoutMutation.isPending} onClick={() => logoutMutation.mutate()} type="button">{logoutMutation.isPending ? "Signing out…" : "Sign out"}</button>
        </div>
        {logoutMutation.isError ? <p className="mt-4 text-sm text-rose-300" role="alert">{friendlyError(logoutMutation.error)}</p> : null}
      </section>

      <section className="mt-8 grid gap-5 lg:grid-cols-[0.72fr_1.28fr]">
        <article className="rounded-2xl border border-sky-200 bg-sky-50 p-5 sm:p-6">
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">Personal namespace</p>
          <p className="mt-4 break-all font-mono text-lg font-semibold text-slate-950">hubcr.io/{user.personal_namespace}</p>
          <p className="mt-3 text-sm leading-6 text-slate-600">This normalized namespace is returned explicitly by the authenticated user API.</p>
        </article>
        <RepositoryPanel namespace={user.personal_namespace} title="Personal repositories" />
      </section>

      <OrganizationWorkspace />
    </div>
  );
}
