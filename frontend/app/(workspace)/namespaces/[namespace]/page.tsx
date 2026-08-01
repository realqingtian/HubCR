import { notFound } from "next/navigation";
import { NamespacePage } from "@/features/namespaces/namespace-page";
import { namespaceNameSchema } from "@/lib/api/schemas";

export default async function Page({ params }: Readonly<{ params: Promise<{ namespace: string }> }>) {
  const { namespace } = await params;
  if (!namespaceNameSchema.safeParse(namespace).success) notFound();
  return <NamespacePage namespace={namespace} />;
}
