"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createTrustPolicyVersion,
  getCurrentTrustPolicy,
  type CreateTrustPolicyRequest,
  type TrustKeylessIdentity,
  type TrustPublicKey,
} from "@/lib/api/client";
import { PanelMessage, useFriendlyError } from "@/features/shared/feedback";
import { useLocale, useT } from "@/lib/i18n";

// TrustPolicyPanel renders the current (highest-version) namespace trust policy and a
// create-new-version form for namespace owners. Read access and create authorization are
// enforced by the backend (ViewOrganization for read, ManageTrustPolicy for create); this
// UI only hides the create form when the read response indicates the viewer is not an
// owner. Re-verification stays informational and never blocks Pull.
export function TrustPolicyPanel({ namespace }: Readonly<{ namespace: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const friendlyError = useFriendlyError();
  const queryClient = useQueryClient();
  const queryKey = ["trust-policy", namespace] as const;
  const policy = useQuery({ queryKey, queryFn: () => getCurrentTrustPolicy(namespace), retry: false });
  const createMutation = useMutation({
    mutationFn: (input: CreateTrustPolicyRequest) => createTrustPolicyVersion(namespace, input),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey }); },
  });

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const keys = readPublicKeyRows(form);
    const identities = readKeylessRows(form);
    createMutation.mutate(
      { public_keys: keys.length > 0 ? keys : undefined, keyless_identities: identities.length > 0 ? identities : undefined },
      { onSuccess: () => form.reset() },
    );
  }

  return (
    <section aria-labelledby="namespace-trust-policy" className="mt-7">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">{t("trustPolicy.eyebrow")}</p>
          <h2 className="mt-2 text-2xl font-semibold tracking-tight text-slate-950" id="namespace-trust-policy">{t("trustPolicy.title")}</h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
            {t("trustPolicy.detail")}
          </p>
        </div>
      </div>

      <div className="mt-5 space-y-3">
        {policy.isPending ? <PanelMessage title={t("trustPolicy.loading")} detail={t("trustPolicy.loadingDetail")} /> : null}
        {policy.isError ? (
          <div className="space-y-3">
            <PanelMessage title={t("trustPolicy.unavailable")} detail={friendlyError(policy.error)} tone="error" />
            <button className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={() => void policy.refetch()} type="button">{t("trustPolicy.retry")}</button>
          </div>
        ) : null}
        {policy.data ? (
          <article className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-mono text-base font-semibold text-slate-950">{t("trustPolicy.version", { n: policy.data.version })}</h3>
              <span className="rounded-full bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-600">{t("trustPolicy.subjects", { n: policy.data.public_keys.length + policy.data.keyless_identities.length, s: (policy.data.public_keys.length + policy.data.keyless_identities.length) === 1 ? "" : "s" })}</span>
            </div>
            <p className="mt-2 text-xs text-slate-400">{t("trustPolicy.created", { date: new Date(policy.data.created_at).toLocaleString(locale) })}</p>
            {policy.data.public_keys.length === 0 && policy.data.keyless_identities.length === 0 ? (
              <p className="mt-3 text-sm text-slate-600">{t("trustPolicy.noSubjects")}</p>
            ) : null}
            {policy.data.public_keys.length > 0 ? (
              <div className="mt-4">
                <p className="text-xs font-semibold uppercase tracking-wide text-slate-600">{t("trustPolicy.publicKeys")}</p>
                <dl className="mt-2 space-y-2">
                  {policy.data.public_keys.map((key) => (
                    <div className="rounded-lg bg-slate-50 px-3 py-2" key={key.fingerprint}>
                      <dt className="text-sm font-medium text-slate-800">{key.name}</dt>
                      <dd className="mt-0.5 break-all font-mono text-xs text-slate-500">{key.fingerprint}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            ) : null}
            {policy.data.keyless_identities.length > 0 ? (
              <div className="mt-4">
                <p className="text-xs font-semibold uppercase tracking-wide text-slate-600">{t("trustPolicy.keylessIdentities")}</p>
                <dl className="mt-2 space-y-2">
                  {policy.data.keyless_identities.map((identity) => (
                    <div className="rounded-lg bg-slate-50 px-3 py-2" key={`${identity.issuer}:${identity.subject}`}>
                      <dt className="break-all text-xs text-slate-500">{identity.issuer}</dt>
                      <dd className="mt-0.5 break-all font-mono text-sm text-slate-800">{identity.subject}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            ) : null}
          </article>
        ) : null}
      </div>

      <TrustPolicyCreateForm namespace={namespace} disabled={createMutation.isPending} onSubmit={submit} error={createMutation.isError ? friendlyError(createMutation.error) : null} />
    </section>
  );
}

function TrustPolicyCreateForm({
  namespace,
  disabled,
  onSubmit,
  error,
}: Readonly<{
  namespace: string;
  disabled: boolean;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
  error: string | null;
}>) {
  const t = useT();
  return (
    <form className="mt-5 space-y-4 rounded-2xl border border-slate-200 bg-white p-5 shadow-sm" onSubmit={onSubmit}>
      <div>
        <p className="text-xs font-semibold uppercase tracking-wide text-slate-600">{t("trustPolicy.form.newKey")}</p>
        <p className="mt-1 text-xs text-slate-500">{t("trustPolicy.form.newKeyNote")}</p>
        <div className="mt-2 grid gap-3 sm:grid-cols-2">
          <div>
            <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-trust-key-name`}>{t("trustPolicy.form.keyName")}</label>
            <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-trust-key-name`} name="key_name" maxLength={128} />
          </div>
          <div>
            <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-trust-key-pem`}>{t("trustPolicy.form.keyPem")}</label>
            <textarea className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 font-mono text-xs outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-trust-key-pem`} name="key_pem" rows={3} />
          </div>
        </div>
      </div>

      <div className="border-t border-slate-100 pt-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-slate-600">{t("trustPolicy.form.newIdentity")}</p>
        <p className="mt-1 text-xs text-slate-500">{t("trustPolicy.form.newIdentityNote")}</p>
        <div className="mt-2 grid gap-3 sm:grid-cols-2">
          <div>
            <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-trust-issuer`}>{t("trustPolicy.form.issuer")}</label>
            <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-trust-issuer`} name="identity_issuer" type="url" inputMode="url" />
          </div>
          <div>
            <label className="text-xs font-semibold uppercase tracking-wide text-slate-600" htmlFor={`${namespace}-trust-subject`}>{t("trustPolicy.form.subject")}</label>
            <input className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-3 py-2.5 text-sm outline-none focus:border-sky-500 focus:ring-4 focus:ring-sky-100" id={`${namespace}-trust-subject`} name="identity_subject" maxLength={2048} />
          </div>
        </div>
      </div>

      {error ? <p className="text-sm text-rose-700" role="alert">{error}</p> : null}
      <button className="rounded-lg bg-slate-950 px-4 py-2.5 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200 disabled:opacity-60" disabled={disabled} type="submit">
        {disabled ? t("trustPolicy.form.submit.pending") : t("trustPolicy.form.submit.default")}
      </button>
    </form>
  );
}

// readPublicKeyRows returns a validated public-key entry when the name and PEM are both
// supplied, or an empty array when the row is left blank (omitted). Invalid input is sent
// to the backend, which returns a 422 with a field-level message.
function readPublicKeyRows(form: HTMLFormElement): TrustPublicKey[] {
  const name = String(new FormData(form).get("key_name") ?? "").trim();
  const pem = String(new FormData(form).get("key_pem") ?? "").trim();
  if (name === "" && pem === "") {
    return [];
  }
  return [{ name, fingerprint: "", public_key_pem: pem }];
}

function readKeylessRows(form: HTMLFormElement): TrustKeylessIdentity[] {
  const issuer = String(new FormData(form).get("identity_issuer") ?? "").trim();
  const subject = String(new FormData(form).get("identity_subject") ?? "").trim();
  if (issuer === "" && subject === "") {
    return [];
  }
  return [{ issuer, subject }];
}
