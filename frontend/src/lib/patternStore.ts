import { writable, derived } from 'svelte/store';
import { patternsApi, type Pattern } from '$lib/api';
export type { Pattern };

const _patterns = writable<Pattern[]>([]);
let _loaded = false;

export async function loadPatterns(courseSlug?: string): Promise<void> {
  if (_loaded && !courseSlug) return;
  try {
    const res = await patternsApi.list(courseSlug);
    _patterns.set(res.patterns ?? []);
    if (!courseSlug) _loaded = true;
  } catch {
    _patterns.set([]);
  }
}

export const patternMap = derived(_patterns, ($p) =>
  Object.fromEntries($p.map((p) => [p.name, p]))
);

export const patterns = { subscribe: _patterns.subscribe };
