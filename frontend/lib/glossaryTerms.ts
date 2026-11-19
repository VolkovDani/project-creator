import { clientCache, clientSessionVersion, query } from './client.ts';
import { type GlossaryFieldsFragment, GlossaryTermsByIdsDocument } from './generated.ts';

const batchLimit = 100;
type Card = GlossaryFieldsFragment | undefined;
type PendingCard = {
  id: string;
  resolve: (term: Card) => void;
  reject: (error: unknown) => void;
};
const pendingBatches = new Map<string, PendingCard[]>();

async function fetchBatch(projectId: string, cards: PendingCard[]) {
  try {
    const result = await query(
      GlossaryTermsByIdsDocument,
      { projectId, ids: cards.map(({ id }) => id) },
      { cache: false },
    );
    const terms = new Map(result.glossaryTermsByIds.map((term) => [term.id, term]));
    for (const card of cards) card.resolve(terms.get(card.id));
  } catch (error) {
    for (const card of cards) card.reject(error);
  }
}

function enqueue(projectId: string, id: string, session: number, generation: number) {
  const key = JSON.stringify([session, generation, projectId]);
  let batch = pendingBatches.get(key);
  if (!batch) {
    const cards: PendingCard[] = [];
    pendingBatches.set(key, cards);
    batch = cards;
    queueMicrotask(() => {
      pendingBatches.delete(key);
      if (session !== clientSessionVersion() || generation !== clientCache.generation) {
        const error = Error('cache reset before fetch');
        for (const card of cards) card.reject(error);
        return;
      }
      for (let i = 0; i < cards.length; i += batchLimit) {
        void fetchBatch(projectId, cards.slice(i, i + batchLimit));
      }
    });
  }
  return new Promise<Card>((resolve, reject) => batch.push({ id, resolve, reject }));
}

export async function loadGlossaryTerms(projectId: string, ids: string[]) {
  const uniqueIds = [...new Set(ids)];
  const tags = [`project:${projectId}:glossary`];
  const session = clientSessionVersion();
  for (;;) {
    const generation = clientCache.generation;
    const revision = clientCache.revision;
    try {
      const cards = await Promise.all(
        uniqueIds.map((id) =>
          clientCache.read(JSON.stringify(['glossary-term', projectId, id]), async () => ({
            data: await enqueue(projectId, id, session, generation),
            tags,
          })),
        ),
      );
      if (
        session === clientSessionVersion() &&
        generation === clientCache.generation &&
        !clientCache.invalidatedSince(revision, tags)
      ) {
        return cards.filter((term) => term !== undefined);
      }
    } catch (error) {
      if (session === clientSessionVersion() && generation === clientCache.generation) throw error;
    }
    if (session !== clientSessionVersion()) throw Error('session changed');
  }
}
