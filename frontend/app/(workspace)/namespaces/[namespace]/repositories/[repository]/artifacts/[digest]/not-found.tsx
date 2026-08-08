import Link from "next/link";

export default function NotFound() {
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
      <p className="text-xs font-semibold uppercase tracking-[0.18em] text-rose-700">Invalid Artifact route</p>
      <h1 className="mt-2 text-2xl font-semibold text-slate-950">Artifact route not found</h1>
      <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">
        Artifact paths require a lowercase SHA-256 Digest in the form <span className="font-mono">sha256:</span> followed by 64 hexadecimal characters.
      </p>
      <Link className="mt-5 inline-flex rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white hover:bg-slate-800 focus:outline-none focus:ring-4 focus:ring-slate-200" href="/">Back to overview</Link>
    </section>
  );
}
