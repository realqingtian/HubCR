export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen w-full max-w-5xl flex-col px-6 py-10 sm:px-10">
      <header className="flex items-center justify-between border-b border-slate-200 pb-6">
        <span className="text-xl font-semibold tracking-tight">HubCR</span>
        <span className="rounded-full bg-sky-50 px-3 py-1 text-xs font-medium text-sky-700">
          Project scaffold
        </span>
      </header>

      <section className="flex flex-1 flex-col justify-center py-20">
        <p className="mb-4 font-mono text-sm text-sky-700">hubcr.io</p>
        <h1 className="max-w-3xl text-4xl font-semibold tracking-tight text-slate-950 sm:text-6xl">
          Open-source OCI registry for teams and organizations.
        </h1>
        <p className="mt-6 max-w-2xl text-lg leading-8 text-slate-600">
          The web application is ready for the first MVP modules. Product flows
          will be added after authentication, namespace, and repository policies
          are confirmed.
        </p>

        <div className="mt-12 grid gap-4 sm:grid-cols-3">
          {[
            ["Control plane", "Go REST API and scoped registry tokens"],
            ["OCI data plane", "CNCF Distribution backed by S3 storage"],
            ["Security workers", "Digest-based scanning and verification"],
          ].map(([title, description]) => (
            <article key={title} className="rounded-2xl border border-slate-200 p-5">
              <h2 className="font-semibold text-slate-900">{title}</h2>
              <p className="mt-2 text-sm leading-6 text-slate-600">{description}</p>
            </article>
          ))}
        </div>
      </section>

      <footer className="border-t border-slate-200 pt-6 text-sm text-slate-500">
        Hub Container Registry
      </footer>
    </main>
  );
}
