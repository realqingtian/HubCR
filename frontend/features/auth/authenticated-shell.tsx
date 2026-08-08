"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, useContext } from "react";
import { APIError, getCurrentUser, logout, type LoginResponse, type User } from "@/lib/api/client";
import { friendlyError, PanelMessage } from "@/features/shared/feedback";
import { LoginPanel } from "./login-panel";
import { currentUserQueryKey, installAuthenticatedUser } from "./query-cache";

const AuthenticatedUserContext = createContext<User | null>(null);

export function useAuthenticatedUser(): User {
  const user = useContext(AuthenticatedUserContext);
  if (user === null) {
    throw new Error("useAuthenticatedUser must be used inside AuthenticatedShell");
  }
  return user;
}

export function AuthenticatedShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const queryClient = useQueryClient();
  const pathname = usePathname();
  const currentUser = useQuery({
    queryKey: currentUserQueryKey,
    queryFn: ({ signal }) => getCurrentUser(signal),
    retry: false,
  });
  const logoutMutation = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.clear();
      void queryClient.invalidateQueries({ queryKey: currentUserQueryKey });
    },
  });

  function loggedIn(result: LoginResponse) {
    installAuthenticatedUser(queryClient, result.user);
  }

  let content: React.ReactNode;
  if (currentUser.isPending) {
    content = <PanelMessage title="Checking your session" detail="Connecting to the HubCR control plane." />;
  } else if (currentUser.isError) {
    if (currentUser.error instanceof APIError && currentUser.error.code === "authentication_failed") {
      content = <LoginPanel onSuccess={loggedIn} />;
    } else {
      content = (
        <div className="space-y-4">
          <PanelMessage title="Control plane unavailable" detail={friendlyError(currentUser.error)} tone="error" />
          <button className="rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={() => void currentUser.refetch()} type="button">Try again</button>
        </div>
      );
    }
  } else {
    const user = currentUser.data;
    content = (
      <AuthenticatedUserContext.Provider value={user}>
        <div className="mb-7 flex flex-col gap-4 rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <nav aria-label="Workspace" className="flex flex-wrap items-center gap-2 text-sm font-medium">
            <WorkspaceLink active={pathname === "/"} href="/">Overview</WorkspaceLink>
            <WorkspaceLink active={pathname.startsWith(`/namespaces/${user.personal_namespace}`)} href={`/namespaces/${user.personal_namespace}`}>Personal namespace</WorkspaceLink>
            <WorkspaceLink active={false} href="/#organizations">Organizations</WorkspaceLink>
          </nav>
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-slate-600">Signed in as <span className="font-semibold text-slate-950">{user.username}</span></p>
            <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100 disabled:opacity-60" disabled={logoutMutation.isPending} onClick={() => logoutMutation.mutate()} type="button">{logoutMutation.isPending ? "Signing out…" : "Sign out"}</button>
          </div>
          {logoutMutation.isError ? <p className="text-sm text-rose-700" role="alert">{friendlyError(logoutMutation.error)}</p> : null}
        </div>
        {children}
      </AuthenticatedUserContext.Provider>
    );
  }

  return (
    <main className="mx-auto min-h-screen w-full max-w-7xl px-5 py-6 sm:px-8 sm:py-8 lg:px-10">
      <header className="mb-8 flex flex-wrap items-center justify-between gap-4 border-b border-slate-200 pb-5">
        <Link className="flex items-center gap-3 rounded-lg focus:outline-none focus:ring-4 focus:ring-sky-100" href="/">
          <span className="grid size-9 place-items-center rounded-xl bg-slate-950 font-mono text-sm font-bold text-white">H</span>
          <span>
            <span className="block text-lg font-semibold tracking-tight text-slate-950">HubCR</span>
            <span className="block text-xs text-slate-500">Container Registry</span>
          </span>
        </Link>
        <span className="rounded-full border border-sky-200 bg-sky-50 px-3 py-1 text-xs font-semibold text-sky-700">Registry MVP · M3 in progress</span>
      </header>
      {content}
      <footer className="mt-10 border-t border-slate-200 py-6 text-xs leading-5 text-slate-500">
        HubCR business control plane · OCI content transfer is provided by CNCF Distribution.
      </footer>
    </main>
  );
}

function WorkspaceLink({ active, children, href }: Readonly<{ active: boolean; children: React.ReactNode; href: string }>) {
  return (
    <Link aria-current={active ? "page" : undefined} className={active ? "rounded-lg bg-slate-950 px-3 py-2 text-white" : "rounded-lg px-3 py-2 text-slate-600 hover:bg-slate-100 hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100"} href={href}>
      {children}
    </Link>
  );
}
