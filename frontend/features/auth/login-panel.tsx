"use client";

import { useMutation } from "@tanstack/react-query";
import { friendlyError } from "@/features/shared/feedback";
import { login, type LoginResponse } from "@/lib/api/client";

export function LoginPanel({ onSuccess }: Readonly<{ onSuccess: (result: LoginResponse) => void }>) {
  const mutation = useMutation({ mutationFn: ({ username, password }: { username: string; password: string }) => login(username, password), onSuccess });

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    mutation.mutate({
      username: String(form.get("username") ?? ""),
      password: String(form.get("password") ?? ""),
    });
  }

  return (
    <section className="grid overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm lg:grid-cols-[1.05fr_0.95fr]">
      <div className="bg-slate-950 px-7 py-10 text-white sm:px-10 sm:py-14">
        <p className="font-mono text-xs uppercase tracking-[0.24em] text-sky-300">HubCR control plane</p>
        <h1 className="mt-5 max-w-xl text-3xl font-semibold tracking-tight sm:text-5xl">
          Registry ownership starts with a clear namespace.
        </h1>
        <p className="mt-5 max-w-lg text-base leading-7 text-slate-300">
          Sign in to manage personal and organization repositories. OCI image transfer remains delegated to CNCF Distribution.
        </p>
        <div className="mt-10 grid gap-3 text-sm text-slate-300 sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
          {[
            ["01", "Identity"],
            ["02", "Ownership"],
            ["03", "Visibility"],
          ].map(([number, label]) => (
            <div key={number} className="rounded-xl border border-white/10 bg-white/5 px-4 py-3">
              <span className="font-mono text-xs text-sky-300">{number}</span>
              <p className="mt-1 font-medium text-white">{label}</p>
            </div>
          ))}
        </div>
      </div>

      <div className="px-7 py-10 sm:px-10 sm:py-14">
        <p className="text-sm font-medium text-sky-700">Administrator invitation accounts</p>
        <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">Sign in to HubCR</h2>
        <p className="mt-2 text-sm leading-6 text-slate-600">
          Public registration is intentionally unavailable in the current self-hosted MVP.
        </p>
        <form className="mt-8 space-y-5" onSubmit={submit}>
          <div>
            <label className="text-sm font-medium text-slate-800" htmlFor="username">Username</label>
            <input className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="username" name="username" autoComplete="username" maxLength={64} required />
          </div>
          <div>
            <label className="text-sm font-medium text-slate-800" htmlFor="password">Password</label>
            <input className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="password" name="password" type="password" autoComplete="current-password" maxLength={1024} required />
          </div>
          {mutation.isError ? <p className="rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{friendlyError(mutation.error)}</p> : null}
          <button className="w-full rounded-xl bg-sky-600 px-4 py-3 text-sm font-semibold text-white transition hover:bg-sky-700 focus:outline-none focus:ring-4 focus:ring-sky-200 disabled:cursor-wait disabled:opacity-60" disabled={mutation.isPending} type="submit">
            {mutation.isPending ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </section>
  );
}
