import type { RepositoryCapabilities, RepositoryVisibility } from "@/lib/api/client";

export function RepositoryQuickStart({
  capabilities,
  namespace,
  repository,
  visibility,
}: Readonly<{
  capabilities: RepositoryCapabilities;
  namespace: string;
  repository: string;
  visibility: RepositoryVisibility;
}>) {
  const image = `hubcr.io/${namespace}/${repository}:TAG`;
  const needsLogin = (visibility === "PRIVATE" && capabilities.can_pull) || capabilities.can_push;

  return (
    <section aria-labelledby="repository-quick-start" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.18em] text-sky-700">Policy-backed commands</p>
          <h2 className="mt-2 text-xl font-semibold text-slate-950" id="repository-quick-start">Quick start</h2>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">
            Commands reflect this Repository&apos;s current Visibility and the Registry actions allowed for your account.
          </p>
        </div>
        <span className="self-start rounded-full bg-slate-100 px-3 py-1.5 text-xs font-semibold text-slate-700">
          {visibility}
        </span>
      </div>

      <div className="mt-6 grid gap-4 lg:grid-cols-3">
        <QuickStartStep title="1. Authenticate">
          <p className="text-sm leading-6 text-slate-600">
            A Web session is not a Registry credential. {needsLogin
              ? "Use your HubCR username and password when Docker prompts."
              : "This public pull does not require Registry login."}
          </p>
          {needsLogin ? <Command value="docker login hubcr.io" /> : null}
        </QuickStartStep>

        <QuickStartStep title="2. Pull">
          {capabilities.can_pull ? (
            <>
              <p className="text-sm leading-6 text-slate-600">Pull the selected Tag through the OCI data plane.</p>
              <Command value={`docker pull ${image}`} />
            </>
          ) : (
            <Unavailable detail="The control plane did not grant Pull for this account." title="Pull access unavailable" />
          )}
        </QuickStartStep>

        <QuickStartStep title="3. Push">
          {capabilities.can_push ? (
            <>
              <p className="text-sm leading-6 text-slate-600">Tag a local image, then Push it to this exact Repository.</p>
              <Command value={`docker tag SOURCE_IMAGE ${image}\ndocker push ${image}`} />
            </>
          ) : (
            <Unavailable detail="Your account may discover this Repository, but policy did not grant Push." title="Push access unavailable" />
          )}
        </QuickStartStep>
      </div>
    </section>
  );
}

function QuickStartStep({ children, title }: Readonly<{ children: React.ReactNode; title: string }>) {
  return (
    <div className="min-w-0 rounded-xl bg-slate-50 p-4">
      <h3 className="text-sm font-semibold text-slate-950">{title}</h3>
      <div className="mt-3 space-y-3">{children}</div>
    </div>
  );
}

function Command({ value }: Readonly<{ value: string }>) {
  return (
    <pre className="overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-100" tabIndex={0}>
      <code>{value}</code>
    </pre>
  );
}

function Unavailable({ detail, title }: Readonly<{ detail: string; title: string }>) {
  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-amber-950" role="status">
      <p className="text-sm font-semibold">{title}</p>
      <p className="mt-1 text-sm leading-6 opacity-80">{detail}</p>
    </div>
  );
}
