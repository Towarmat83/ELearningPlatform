// ─────────────────────────────────────────────────────────────────────────────
// Lab Library — localStorage-backed store of saved LabExport templates
// ─────────────────────────────────────────────────────────────────────────────

import { writable, get } from 'svelte/store';
import type { LabExport } from './labCodec';

export interface LibraryEntry {
  id: string;
  name: string;
  description: string;
  type: 'form' | 'ctf' | 'interactive';
  savedAt: string;
  lab: LabExport;
}

const STORAGE_KEY = 'elearning_lab_library_v1';

function readStorage(): LibraryEntry[] {
  if (typeof localStorage === 'undefined') return [];
  try { return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]'); }
  catch { return []; }
}

function writeStorage(items: LibraryEntry[]) {
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  }
}

function createStore() {
  const { subscribe, set, update } = writable<LibraryEntry[]>([]);

  return {
    subscribe,
    /** Must be called in onMount to hydrate from localStorage */
    init() { set(readStorage()); },

    add(lab: LabExport, name?: string, description?: string): LibraryEntry {
      const entry: LibraryEntry = {
        id: `lib_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
        name: name?.trim() || lab.title,
        description: description?.trim() ?? lab.description?.slice(0, 140) ?? '',
        type: lab.type,
        savedAt: new Date().toISOString(),
        lab,
      };
      update(items => { const n = [entry, ...items]; writeStorage(n); return n; });
      return entry;
    },

    remove(id: string) {
      update(items => { const n = items.filter(e => e.id !== id); writeStorage(n); return n; });
    },

    rename(id: string, name: string, description: string) {
      update(items => {
        const n = items.map(e => e.id === id ? { ...e, name: name.trim() || e.name, description } : e);
        writeStorage(n); return n;
      });
    },

    exportJSON(): string { return JSON.stringify(get({ subscribe }), null, 2); },

    importJSON(json: string) {
      const parsed = JSON.parse(json);
      if (!Array.isArray(parsed)) throw new Error('Expected a JSON array of library entries');
      writeStorage(parsed as LibraryEntry[]);
      set(parsed as LibraryEntry[]);
    },
  };
}

export const labLibrary = createStore();
