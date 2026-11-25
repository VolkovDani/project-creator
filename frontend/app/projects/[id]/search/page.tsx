import { redirect } from 'next/navigation';
export default async function SearchPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/projects/${encodeURIComponent(id)}/requirements#requirement-search`);
}
