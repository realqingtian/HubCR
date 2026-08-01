import { notFound } from "next/navigation";
import { RepositoryPage } from "@/features/repositories/repository-page";
import { namespaceNameSchema } from "@/lib/api/schemas";

export default async function Page({ params }: Readonly<{ params: Promise<{ namespace: string; repository: string }> }>) {
  const { namespace, repository } = await params;
  if (!namespaceNameSchema.safeParse(namespace).success || !namespaceNameSchema.safeParse(repository).success) notFound();
  return <RepositoryPage namespace={namespace} repository={repository} />;
}
