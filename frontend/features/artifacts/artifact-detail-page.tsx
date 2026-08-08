"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { ArtifactSecurityPanel } from "@/features/security/artifact-security-panel";
import { friendlyError, PanelMessage } from "@/features/shared/feedback";
import { APIError, getArtifact, type Artifact, type ManifestDescriptor } from "@/lib/api/client";

export function ArtifactDetailPage({ digest, namespace, repository }: Readonly<{ digest: string; namespace: string; repository: string }>) {
  const detail = useQuery({
    queryKey: ["repositories", namespace, repository, "artifacts", digest],
    queryFn: () => getArtifact(namespace, repository, digest),
    retry: false,
  });

  return (
    <div>
      <nav aria-label="Breadcrumb" className="text-sm text-slate-500">
        <ol className="flex flex-wrap items-center gap-2">
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href="/">Overview</Link></li>
          <li aria-hidden="true">/</li>
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href={`/namespaces/${namespace}`}>{namespace}</Link></li>
          <li aria-hidden="true">/</li>
          <li><Link className="rounded hover:text-slate-950 focus:outline-none focus:ring-4 focus:ring-slate-100" href={`/namespaces/${namespace}/repositories/${repository}`}>{repository}</Link></li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-slate-800">Artifact</li>
        </ol>
      </nav>

      <div className="mt-5">
        {detail.isPending ? <PanelMessage title="Loading Artifact" detail="Reading immutable Digest metadata from the control plane." /> : null}
        {detail.isError ? <ArtifactFailure error={detail.error} retry={() => void detail.refetch()} /> : null}
        {detail.data ? <ArtifactDetail artifact={detail.data} namespace={namespace} repository={repository} /> : null}
      </div>
    </div>
  );
}

function ArtifactFailure({ error, retry }: Readonly<{ error: unknown; retry: () => void }>) {
  const missing = error instanceof APIError && error.code === "not_found";
  return (
    <div className="space-y-3">
      <PanelMessage title={missing ? "Artifact not found" : "Artifact unavailable"} detail={missing ? "The Digest does not exist in this repository or your session cannot discover it." : friendlyError(error)} tone="error" />
      {!missing ? <button className="rounded-lg border border-slate-300 px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-4 focus:ring-slate-100" onClick={retry} type="button">Try again</button> : null}
    </div>
  );
}

function ArtifactDetail({ artifact, namespace, repository }: Readonly<{ artifact: Artifact; namespace: string; repository: string }>) {
  return (
    <div className="space-y-6">
      <section className="rounded-3xl bg-slate-950 px-6 py-7 text-white shadow-sm sm:px-8">
        <div className="flex flex-wrap items-center gap-3">
          <span className="rounded-full bg-sky-400/15 px-3 py-1 text-xs font-semibold text-sky-200">{artifact.kind}</span>
          <span className="text-xs text-slate-400">Immutable Artifact</span>
        </div>
        <h1 className="mt-4 break-all font-mono text-lg font-semibold leading-8 sm:text-2xl">{artifact.digest}</h1>
        <p className="mt-3 text-sm leading-6 text-slate-300">This Digest is the Artifact identity. Tags may move without changing this record.</p>
      </section>

      <section aria-labelledby="artifact-facts" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
        <h2 className="text-lg font-semibold text-slate-950" id="artifact-facts">Artifact facts</h2>
        <dl className="mt-5 grid gap-4 sm:grid-cols-2">
          <Fact label="Media type" value={artifact.media_type ?? "Unavailable"} mono />
          <Fact label="Size" value={artifact.size_bytes === undefined ? "Unavailable" : `${artifact.size_bytes} bytes`} />
          <Fact label="Source created" value={artifact.source_created_at ? new Date(artifact.source_created_at).toLocaleString() : "Unavailable"} />
          <Fact label="Discovered" value={new Date(artifact.discovered_at).toLocaleString()} />
        </dl>
      </section>

      <DescriptorSection artifact={artifact} />
      <ArtifactSecurityPanel digest={artifact.digest} namespace={namespace} repository={repository} />
    </div>
  );
}

function DescriptorSection({ artifact }: Readonly<{ artifact: Artifact }>) {
  if (artifact.kind !== "INDEX") {
    return <PanelMessage title="Manifest Artifact" detail="Ordered child Manifest descriptors apply only to Index Artifacts." />;
  }
  if (!artifact.descriptors_complete) {
    return <PanelMessage title="Descriptor set unavailable" detail="The control plane has not confirmed the complete ordered child Manifest set for this Index." />;
  }
  if (artifact.manifests?.length === 0) {
    return <PanelMessage title="Confirmed empty descriptor set" detail="The control plane confirms that this Index has no child Manifest descriptors." />;
  }
  const manifests = artifact.manifests ?? [];
  return (
    <section aria-labelledby="index-manifests" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
      <h2 className="text-lg font-semibold text-slate-950" id="index-manifests">Ordered child Manifests</h2>
      <div className="mt-4 space-y-3">
        {manifests.map((manifest) => <ManifestCard key={`${manifest.position}:${manifest.digest}`} manifest={manifest} />)}
      </div>
    </section>
  );
}

function ManifestCard({ manifest }: Readonly<{ manifest: ManifestDescriptor }>) {
  return (
    <article className="rounded-xl bg-slate-50 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-slate-900">Position {manifest.position}</h3>
        <span className="text-xs text-slate-500">{manifest.size_bytes === undefined ? "Size unavailable" : `${manifest.size_bytes} bytes`}</span>
      </div>
      <p className="mt-3 break-all font-mono text-xs leading-5 text-slate-700">{manifest.digest}</p>
      <p className="mt-2 break-all text-xs text-slate-500">{manifest.media_type ?? "Media type unavailable"}</p>
      <p className="mt-2 text-xs font-medium text-slate-700">{manifest.platform ? `${manifest.platform.os}/${manifest.platform.architecture}${manifest.platform.variant ? `/${manifest.platform.variant}` : ""}` : "Platform unavailable"}</p>
    </article>
  );
}

function Fact({ label, mono = false, value }: Readonly<{ label: string; mono?: boolean; value: string }>) {
  return (
    <div className="rounded-xl bg-slate-50 p-4">
      <dt className="text-xs font-semibold uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className={`mt-2 break-all text-sm text-slate-900 ${mono ? "font-mono" : ""}`}>{value}</dd>
    </div>
  );
}
