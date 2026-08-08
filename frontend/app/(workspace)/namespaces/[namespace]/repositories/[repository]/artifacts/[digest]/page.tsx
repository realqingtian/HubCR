import { notFound } from "next/navigation";
import { ArtifactDetailPage } from "@/features/artifacts/artifact-detail-page";
import { artifactDigestSchema, namespaceNameSchema } from "@/lib/api/schemas";

export default async function Page({ params }: Readonly<{
  params: Promise<{ digest: string; namespace: string; repository: string }>;
}>) {
  const { digest, namespace, repository } = await params;
  const decodedDigest = decodeRouteSegment(digest);
  if (
    !namespaceNameSchema.safeParse(namespace).success ||
    !namespaceNameSchema.safeParse(repository).success ||
    decodedDigest === undefined ||
    !artifactDigestSchema.safeParse(decodedDigest).success
  ) {
    notFound();
  }
  return <ArtifactDetailPage digest={decodedDigest} namespace={namespace} repository={repository} />;
}

function decodeRouteSegment(value: string): string | undefined {
  try {
    return decodeURIComponent(value);
  } catch {
    return undefined;
  }
}
