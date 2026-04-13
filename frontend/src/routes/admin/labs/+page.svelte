<script lang="ts">
  import { onMount } from 'svelte';
  import { auth, toasts } from '$lib/stores';
  import { labLibrary, type LibraryEntry } from '$lib/labLibrary';
  import {
    validateLabJSON, parseMarkdownToLab, type LabExport,
    INTERACTIVE_JSON_TEMPLATE, FORM_JSON_TEMPLATE, CTF_JSON_TEMPLATE,
    INTERACTIVE_MD_TEMPLATE, FORM_MD_TEMPLATE, CTF_MD_TEMPLATE, CTF_MULTI_MD_TEMPLATE,
  } from '$lib/labCodec';

  // ─── Tabs ─────────────────────────────────────────────────────────────────
  let activeTab: 'library' | 'markdown' = 'library';

  // ─── Library ──────────────────────────────────────────────────────────────
  let filterType: 'all' | 'form' | 'ctf' | 'interactive' = 'all';
  let searchQuery = '';
  let addFormOpen = false;
  let addName = '';
  let addJson = '';
  let addJsonError = '';
  let renamingId: string | null = null;
  let renameValues: Record<string, { name: string; description: string }> = {};
  let expandedId: string | null = null;

  $: filteredEntries = $labLibrary.filter(e => {
    if (filterType !== 'all' && e.type !== filterType) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return e.name.toLowerCase().includes(q) || e.description.toLowerCase().includes(q);
    }
    return true;
  });

  function addToLibrary() {
    addJsonError = '';
    try {
      const parsed = JSON.parse(addJson);
      const validated = validateLabJSON(parsed);
      labLibrary.add(validated, addName || undefined);
      addName = '';
      addJson = '';
      addFormOpen = false;
      toasts.success('Template saved to library');
    } catch (e: any) {
      addJsonError = e.message ?? 'Invalid JSON';
    }
  }

  function deleteEntry(id: string) {
    if (!confirm('Remove this template from the library?')) return;
    labLibrary.remove(id);
    toasts.success('Removed');
  }

  function copyJSON(entry: LibraryEntry) {
    navigator.clipboard.writeText(JSON.stringify(entry.lab, null, 2)).then(() => {
      toasts.success('JSON copied to clipboard');
    });
  }

  function exportEntry(entry: LibraryEntry) {
    const blob = new Blob([JSON.stringify(entry.lab, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${entry.name.replace(/[^a-z0-9]/gi, '_').toLowerCase()}.lab.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  function startRename(entry: LibraryEntry) {
    renamingId = entry.id;
    renameValues[entry.id] = { name: entry.name, description: entry.description };
  }

  function confirmRename(id: string) {
    const v = renameValues[id];
    if (v) labLibrary.rename(id, v.name, v.description);
    renamingId = null;
  }

  // ─── Library bulk export/import ───────────────────────────────────────────
  let importLibJson = '';
  let importLibError = '';
  let importLibOpen = false;

  function exportLibrary() {
    const blob = new Blob([labLibrary.exportJSON()], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = 'lab_library.json'; a.click();
    URL.revokeObjectURL(url);
  }

  function importLibrary() {
    importLibError = '';
    try {
      labLibrary.importJSON(importLibJson);
      importLibJson = '';
      importLibOpen = false;
      toasts.success('Library imported');
    } catch (e: any) {
      importLibError = e.message ?? 'Invalid format';
    }
  }

  // ─── Markdown converter ───────────────────────────────────────────────────
  let mdInput = '';
  let mdResult: LabExport | null = null;
  let mdResultText = '';
  let mdError = '';
  let mdTemplate: 'interactive' | 'form' | 'ctf' | 'ctf-multi' = 'interactive';
  let showFormatRef = false;
  let saveToLibName = '';
  let savingToLib = false;

  function getMdTemplate() {
    if (mdTemplate === 'form') return FORM_MD_TEMPLATE;
    if (mdTemplate === 'ctf') return CTF_MD_TEMPLATE;
    if (mdTemplate === 'ctf-multi') return CTF_MULTI_MD_TEMPLATE;
    return INTERACTIVE_MD_TEMPLATE;
  }

  function copyTemplate() {
    mdInput = getMdTemplate();
    mdResult = null; mdError = '';
  }

  function convert() {
    mdError = ''; mdResult = null; mdResultText = '';
    try {
      const r = parseMarkdownToLab(mdInput);
      mdResult = r;
      mdResultText = JSON.stringify(r, null, 2);
    } catch (e: any) {
      mdError = e.message ?? 'Parse error';
    }
  }

  function copyResult() {
    if (!mdResultText) return;
    navigator.clipboard.writeText(mdResultText).then(() => toasts.success('JSON copied'));
  }

  function saveResultToLibrary() {
    if (!mdResult) return;
    labLibrary.add(mdResult, saveToLibName || undefined);
    saveToLibName = '';
    toasts.success(`"${mdResult.title}" saved to library`);
  }

  function setFilterType(v: string) {
    filterType = v as typeof filterType;
  }

  function setMdTemplate(v: string) {
    mdTemplate = v as typeof mdTemplate;
  }

  // ─── Lifecycle ────────────────────────────────────────────────────────────
  onMount(() => { labLibrary.init(); });

  function typeLabel(t: string) {
    if (t === 'ctf') return '🚩 CTF';
    if (t === 'interactive') return '⚡ Interactive';
    return '📝 Quiz';
  }

  function typeBadge(t: string) {
    if (t === 'ctf') return 'badge-ctf';
    if (t === 'interactive') return 'badge-blue';
    return 'badge-form';
  }

  function relativeDate(iso: string) {
    const d = new Date(iso);
    return d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'short', year: 'numeric' });
  }

  function labSummary(entry: LibraryEntry): string {
    const c = entry.lab.content;
    if (entry.type === 'form') return `${c.questions?.length ?? 0} question(s)`;
    if (entry.type === 'interactive') return `${c.steps?.length ?? 0} step(s) · ${c.docker_image ?? ''}`;
    if (c.flags?.length) return `${c.flags.length} flag(s) · multi`;
    return `single flag · ${c.category ?? 'misc'}`;
  }
</script>

<svelte:head><title>Lab Tools — Admin</title></svelte:head>

<div class="p-8 max-w-6xl mx-auto">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-2xl font-bold text-gray-800">Lab Tools</h1>
  </div>

  <!-- ── Tab bar ──────────────────────────────────────────────────────── -->
  <div class="flex gap-1 mb-6 border-b border-gray-200">
    <button
      class="px-5 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors
        {activeTab === 'library' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
      on:click={() => activeTab = 'library'}>
      Lab Library
    </button>
    <button
      class="px-5 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors
        {activeTab === 'markdown' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
      on:click={() => activeTab = 'markdown'}>
      Markdown → JSON
    </button>
  </div>

  <!-- ══════════════════════════════════════════════════════════════════════
       LIBRARY TAB
  ══════════════════════════════════════════════════════════════════════ -->
  {#if activeTab === 'library'}
    <!-- Toolbar -->
    <div class="flex flex-wrap items-center gap-3 mb-5">
      <!-- Filters -->
      <div class="flex gap-1 bg-gray-100 rounded-lg p-1">
        {#each [['all','All'], ['form','📝 Quiz'], ['ctf','🚩 CTF'], ['interactive','⚡ Interactive']] as [val, label]}
          <button
            class="text-xs px-3 py-1.5 rounded-md font-medium transition-colors
              {filterType === val ? 'bg-white text-gray-800 shadow-sm' : 'text-gray-500 hover:text-gray-700'}"
            on:click={() => setFilterType(val)}>
            {label}
          </button>
        {/each}
      </div>

      <!-- Search -->
      <input
        class="input text-sm flex-1 max-w-xs"
        bind:value={searchQuery}
        placeholder="Search templates..."
      />

      <div class="ml-auto flex gap-2">
        <button class="btn-secondary text-xs" on:click={exportLibrary} disabled={$labLibrary.length === 0}>
          ↓ Export library
        </button>
        <button class="btn-secondary text-xs" on:click={() => importLibOpen = !importLibOpen}>
          ↑ Import library
        </button>
        <button class="btn-primary text-xs" on:click={() => addFormOpen = !addFormOpen}>
          {addFormOpen ? '− Cancel' : '+ Add template'}
        </button>
      </div>
    </div>

    <!-- Import library form -->
    {#if importLibOpen}
      <div class="bg-amber-50 border border-amber-200 rounded-xl p-4 mb-4 space-y-2">
        <p class="text-xs text-amber-700 font-medium">Paste a library export JSON (array of entries). This will <strong>replace</strong> the current library.</p>
        <textarea class="input font-mono text-xs" rows="4" bind:value={importLibJson} placeholder="[...]"></textarea>
        {#if importLibError}
          <p class="text-xs text-red-600">{importLibError}</p>
        {/if}
        <button class="btn-primary text-xs" on:click={importLibrary}>Import & replace</button>
      </div>
    {/if}

    <!-- Add template form -->
    {#if addFormOpen}
      <div class="bg-primary-50 border border-primary-200 rounded-xl p-4 mb-4 space-y-3">
        <h3 class="text-sm font-semibold text-primary-800">Add template from JSON</h3>
        <div>
          <label class="label text-xs">Name <span class="text-gray-400 font-normal">(optional — uses lab title by default)</span></label>
          <input class="input text-sm" bind:value={addName} placeholder="My template name" />
        </div>
        <div>
          <label class="label text-xs">Lab JSON *</label>
          <textarea class="input font-mono text-xs leading-relaxed" rows="12" bind:value={addJson}
            placeholder={`{\n  "version": "1.0",\n  "type": "interactive",\n  ...\n}`}
          ></textarea>
        </div>
        {#if addJsonError}
          <div class="bg-red-50 border border-red-200 rounded-lg px-3 py-2 text-xs text-red-600 font-mono">{addJsonError}</div>
        {/if}
        <div class="flex gap-2">
          <button class="btn-primary text-xs" on:click={addToLibrary} disabled={!addJson.trim()}>Save to library</button>
          <button class="btn-secondary text-xs" on:click={() => { addFormOpen = false; addJson = ''; addName = ''; addJsonError = ''; }}>Cancel</button>
        </div>
      </div>
    {/if}

    <!-- Entry count -->
    {#if $labLibrary.length > 0}
      <p class="text-xs text-gray-400 mb-3">
        {filteredEntries.length} of {$labLibrary.length} template{$labLibrary.length !== 1 ? 's' : ''}
      </p>
    {/if}

    <!-- Cards -->
    {#if filteredEntries.length === 0}
      <div class="text-center py-16 text-gray-400">
        {#if $labLibrary.length === 0}
          <div class="text-4xl mb-3">📦</div>
          <p class="font-medium">No templates yet</p>
          <p class="text-sm mt-1">Add a JSON template or convert a Markdown lab to get started.</p>
        {:else}
          <p>No templates match your filter.</p>
        {/if}
      </div>
    {:else}
      <div class="space-y-3">
        {#each filteredEntries as entry (entry.id)}
          <div class="bg-white rounded-xl border border-gray-200 shadow-sm overflow-hidden transition-shadow hover:shadow-md">
            <div class="p-4">
              <!-- Header row -->
              <div class="flex items-start gap-3">
                <div class="flex-1 min-w-0">
                  {#if renamingId === entry.id}
                    <div class="space-y-2 mb-2">
                      <input
                        class="input text-sm font-semibold"
                        bind:value={renameValues[entry.id].name}
                        placeholder="Template name"
                      />
                      <input
                        class="input text-xs"
                        bind:value={renameValues[entry.id].description}
                        placeholder="Short description..."
                      />
                      <div class="flex gap-2">
                        <button class="btn-primary text-xs" on:click={() => confirmRename(entry.id)}>Save</button>
                        <button class="btn-secondary text-xs" on:click={() => renamingId = null}>Cancel</button>
                      </div>
                    </div>
                  {:else}
                    <div class="flex items-center gap-2 mb-1 flex-wrap">
                      <span class="{typeBadge(entry.type)} text-xs">{typeLabel(entry.type)}</span>
                      <span class="font-semibold text-gray-800 text-sm">{entry.name}</span>
                      <span class="text-xs text-gray-400">{labSummary(entry)}</span>
                    </div>
                    {#if entry.description}
                      <p class="text-xs text-gray-500 line-clamp-2">{entry.description}</p>
                    {/if}
                    <p class="text-xs text-gray-300 mt-1">Saved {relativeDate(entry.savedAt)} · {entry.lab.points} pts</p>
                  {/if}
                </div>

                <!-- Actions -->
                {#if renamingId !== entry.id}
                  <div class="flex items-center gap-1 shrink-0 flex-wrap justify-end">
                    <button
                      class="text-xs px-2 py-1 rounded bg-gray-100 text-gray-600 hover:bg-gray-200 transition-colors"
                      on:click={() => expandedId = expandedId === entry.id ? null : entry.id}
                      title="Preview JSON">
                      {expandedId === entry.id ? '▲ Hide' : '▼ Preview'}
                    </button>
                    <button
                      class="text-xs px-2 py-1 rounded bg-gray-100 text-gray-600 hover:bg-gray-200 transition-colors"
                      on:click={() => copyJSON(entry)}
                      title="Copy JSON">
                      Copy JSON
                    </button>
                    <button
                      class="text-xs px-2 py-1 rounded bg-gray-100 text-gray-600 hover:bg-gray-200 transition-colors"
                      on:click={() => exportEntry(entry)}
                      title="Download as file">
                      ↓ Export
                    </button>
                    <button
                      class="text-xs px-2 py-1 rounded bg-gray-100 text-gray-600 hover:bg-gray-200 transition-colors"
                      on:click={() => startRename(entry)}
                      title="Edit name">
                      ✏ Rename
                    </button>
                    <button
                      class="text-xs px-2 py-1 rounded bg-red-50 text-red-500 hover:bg-red-100 transition-colors"
                      on:click={() => deleteEntry(entry.id)}
                      title="Delete">
                      × Delete
                    </button>
                  </div>
                {/if}
              </div>

              <!-- JSON Preview (expandable) -->
              {#if expandedId === entry.id}
                <div class="mt-3 pt-3 border-t border-gray-100">
                  <textarea
                    class="input font-mono text-xs leading-relaxed bg-gray-50"
                    rows="12"
                    readonly
                    value={JSON.stringify(entry.lab, null, 2)}
                  ></textarea>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}

  <!-- ══════════════════════════════════════════════════════════════════════
       MARKDOWN → JSON TAB
  ══════════════════════════════════════════════════════════════════════ -->
  {:else if activeTab === 'markdown'}
    <!-- Template selector bar -->
    <div class="flex items-center gap-3 mb-4 flex-wrap">
      <span class="text-sm text-gray-500 font-medium">Template:</span>
      <div class="flex gap-1 bg-gray-100 rounded-lg p-1">
        {#each [['interactive','⚡ Interactive'], ['form','📝 Quiz'], ['ctf','🚩 CTF'], ['ctf-multi','🚩 CTF Multi']] as [val, label]}
          <button
            class="text-xs px-3 py-1.5 rounded-md font-medium transition-colors
              {mdTemplate === val ? 'bg-white text-gray-800 shadow-sm' : 'text-gray-500 hover:text-gray-700'}"
            on:click={() => setMdTemplate(val)}>
            {label}
          </button>
        {/each}
      </div>
      <button class="btn-secondary text-xs" on:click={copyTemplate}>Load template</button>
    </div>

    <!-- Split panel -->
    <div class="grid md:grid-cols-2 gap-4 mb-4">
      <!-- Left: Markdown input -->
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <label class="label mb-0 text-sm">Markdown Input</label>
          <span class="text-xs text-gray-400">{mdInput.length} chars</span>
        </div>
        <textarea
          class="input font-mono text-xs leading-relaxed flex-1 resize-none"
          rows="28"
          bind:value={mdInput}
          placeholder="Paste your Markdown here or load a template above..."
          spellcheck="false"
        ></textarea>
        <button
          class="btn-primary text-sm"
          on:click={convert}
          disabled={!mdInput.trim()}>
          Convert →
        </button>
      </div>

      <!-- Right: JSON output -->
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <label class="label mb-0 text-sm">JSON Output</label>
          {#if mdResult}
            <span class="{typeBadge(mdResult.type)} text-xs">{typeLabel(mdResult.type)}</span>
          {/if}
        </div>

        {#if mdError}
          <div class="bg-red-50 border border-red-200 rounded-lg p-3 flex-1">
            <p class="text-sm font-semibold text-red-700 mb-1">Parse error</p>
            <p class="text-xs text-red-600 font-mono">{mdError}</p>
          </div>
        {:else if mdResult}
          <textarea
            class="input font-mono text-xs leading-relaxed flex-1 resize-none bg-gray-50"
            rows="24"
            readonly
            value={mdResultText}
          ></textarea>
          <!-- Save actions -->
          <div class="bg-green-50 border border-green-200 rounded-xl p-3 space-y-2">
            <p class="text-xs font-semibold text-green-700">Converted successfully — {mdResult.title}</p>
            <div class="flex gap-2 flex-wrap">
              <button class="btn-secondary text-xs" on:click={copyResult}>Copy JSON</button>
              <div class="flex gap-1 flex-1">
                <input
                  class="input text-xs flex-1"
                  bind:value={saveToLibName}
                  placeholder="Template name (optional)"
                />
                <button class="btn-primary text-xs whitespace-nowrap" on:click={saveResultToLibrary}>
                  + Save to Library
                </button>
              </div>
            </div>
          </div>
        {:else}
          <div class="input bg-gray-50 flex-1 flex items-center justify-center min-h-[200px] text-gray-300 text-sm">
            JSON output will appear here
          </div>
        {/if}
      </div>
    </div>

    <!-- Format reference -->
    <div class="border border-gray-200 rounded-xl overflow-hidden">
      <button
        type="button"
        class="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-gray-600 bg-gray-50 hover:bg-gray-100 transition-colors"
        on:click={() => showFormatRef = !showFormatRef}>
        <span>Format reference</span>
        <span class="text-gray-400">{showFormatRef ? '▲' : '▼'}</span>
      </button>
      {#if showFormatRef}
        <div class="p-4 space-y-4 text-xs text-gray-600">
          <div class="grid md:grid-cols-2 gap-4">
            <div class="space-y-2">
              <p class="font-semibold text-gray-800">Common frontmatter</p>
              <pre class="bg-gray-100 rounded-lg p-3 text-xs overflow-x-auto">---
title: Lab Title        (required)
type: interactive       (required: form | ctf | interactive)
points: 100             (optional, default 100)
description: ...        (optional)
is_published: false     (optional)
order_index: 0          (optional)
---</pre>
            </div>
            <div class="space-y-2">
              <p class="font-semibold text-gray-800">Interactive lab</p>
              <pre class="bg-gray-100 rounded-lg p-3 text-xs overflow-x-auto">---
type: interactive
docker_image: ubuntu:22.04  (required)
---
## Step Title
Description text.

```bash
pwd # explanation
ls -la # list files
```</pre>
            </div>
            <div class="space-y-2">
              <p class="font-semibold text-gray-800">Form quiz</p>
              <pre class="bg-gray-100 rounded-lg p-3 text-xs overflow-x-auto">---
type: form
---
## Question text
- [x] Correct answer
- [ ] Wrong option
> Explanation (optional)

## Free text question
&lt;!-- type: text --&gt;
**Answer:** correct answer</pre>
            </div>
            <div class="space-y-2">
              <p class="font-semibold text-gray-800">CTF challenge</p>
              <pre class="bg-gray-100 rounded-lg p-3 text-xs overflow-x-auto">---
type: ctf
flag: FLAG&#123;secret&#125;    (required)
category: web
---
Challenge description.
## Hints
- Hint 1
## Resources
- [Name](url)

# Multi-flag: add mode: multi
# plus ## Flags section with
# ### Flag Name (100 pts)
# flag: FLAG&#123;value&#125;</pre>
            </div>
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>
