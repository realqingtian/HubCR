"use client";

import { useMutation } from "@tanstack/react-query";
import { useFriendlyError } from "@/features/shared/feedback";
import { LanguageSwitcher } from "@/features/i18n/language-switcher";
import { useT } from "@/lib/i18n";
import { login, type LoginResponse } from "@/lib/api/client";

export function LoginPanel({ onSuccess }: Readonly<{ onSuccess: (result: LoginResponse) => void }>) {
  const t = useT();
  const friendlyError = useFriendlyError();
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
        <div className="flex items-center justify-between gap-3">
          <p className="font-mono text-xs uppercase tracking-[0.24em] text-sky-300">{t("login.brand")}</p>
          <LanguageSwitcher />
        </div>
        <h1 className="mt-5 max-w-xl text-3xl font-semibold tracking-tight sm:text-5xl">
          {t("login.title")}
        </h1>
        <p className="mt-5 max-w-lg text-base leading-7 text-slate-300">
          {t("login.subtitle")}
        </p>
        <div className="mt-10 grid gap-3 text-sm text-slate-300 sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
          {[
            ["01", t("login.pillar.identity")],
            ["02", t("login.pillar.ownership")],
            ["03", t("login.pillar.visibility")],
          ].map(([number, label]) => (
            <div key={number} className="rounded-xl border border-white/10 bg-white/5 px-4 py-3">
              <span className="font-mono text-xs text-sky-300">{number}</span>
              <p className="mt-1 font-medium text-white">{label}</p>
            </div>
          ))}
        </div>
      </div>

      <div className="px-7 py-10 sm:px-10 sm:py-14">
        <p className="text-sm font-medium text-sky-700">{t("login.invitation")}</p>
        <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950">{t("login.heading")}</h2>
        <p className="mt-2 text-sm leading-6 text-slate-600">
          {t("login.registrationNote")}
        </p>
        <form className="mt-8 space-y-5" onSubmit={submit}>
          <div>
            <label className="text-sm font-medium text-slate-800" htmlFor="username">{t("login.username")}</label>
            <input className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="username" name="username" autoComplete="username" maxLength={64} required />
          </div>
          <div>
            <label className="text-sm font-medium text-slate-800" htmlFor="password">{t("login.password")}</label>
            <input className="mt-2 w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm outline-none transition focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id="password" name="password" type="password" autoComplete="current-password" maxLength={1024} required />
          </div>
          {mutation.isError ? <p className="rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{friendlyError(mutation.error)}</p> : null}
          <button className="w-full rounded-xl bg-sky-600 px-4 py-3 text-sm font-semibold text-white transition hover:bg-sky-700 focus:outline-none focus:ring-4 focus:ring-sky-200 disabled:cursor-wait disabled:opacity-60" disabled={mutation.isPending} type="submit">
            {mutation.isPending ? t("login.submit.pending") : t("login.submit.default")}
          </button>
        </form>
      </div>
    </section>
  );
}
