<script lang="ts">
  import { onMount } from 'svelte';
  import { patternsApi, type Pattern } from '$lib/api';
  import { auth, toasts } from '$lib/stores';
  import Markdown from '$lib/Markdown.svelte';

  let patterns: Pattern[] = [];
  let loading = true;
  let creating = false;
  let saving = false;

  // CRD YAML modal
  let crdYaml = '';
  let showCrdModal = false;

  // New / edit pattern form
  let draft: Pattern | null = null;
  let editingName: string | null = null; // non-null = edit mode
  let prevName = '';

  const emptyDraft = (): Pattern => ({
    name: '', label: '', scope: 'global',
    html: '<span class="md-pat-{{name}}">{{content}}</span>',
    css: '.md-pat-{{name}} {\n  \n}',
    js: '',
    source: 'crd',
  });

  function onNameChange(name: string) {
    if (!draft) return;
    const from = prevName ? `md-pat-${prevName}` : 'md-pat-{{name}}';
    const to   = name    ? `md-pat-${name}`      : 'md-pat-{{name}}';
    if (draft.html.includes(from)) draft.html = draft.html.replaceAll(from, to);
    if (draft.css.includes(from))  draft.css  = draft.css.replaceAll(from, to);
    prevName = name;
  }

  function startDraft() {
    draft = emptyDraft();
    editingName = null;
    prevName = '';
  }

  function startEdit(p: Pattern) {
    draft = { ...p };
    editingName = p.name;
    prevName = p.name;
    customPreviewMd = '';
  }

  onMount(loadPatterns);

  async function loadPatterns() {
    loading = true;
    try {
      const res = await patternsApi.list();
      patterns = res.patterns ?? [];
    } catch (e: any) {
      toasts.error(e.message || 'Failed to load patterns');
    } finally {
      loading = false;
    }
  }

  function generateCRD(p: Pattern): string {
    const esc = (s: string) =>
      s.includes('\n')
        ? `|\n${s.split('\n').map(l => '    ' + l).join('\n')}`
        : JSON.stringify(s);
    return [
      `apiVersion: elearning.example.com/v1`,
      `kind: MarkdownPattern`,
      `metadata:`,
      `  name: ${p.name}`,
      `  namespace: default`,
      `spec:`,
      `  name: ${p.name}`,
      p.label ? `  label: ${JSON.stringify(p.label)}` : null,
      `  scope: ${p.scope || 'global'}`,
      `  html: ${esc(p.html)}`,
      p.css ? `  css: ${esc(p.css)}` : null,
      p.js  ? `  js: ${esc(p.js)}`  : null,
    ].filter(Boolean).join('\n');
  }

  function openCrdModal(p: Pattern) {
    crdYaml = generateCRD(p);
    showCrdModal = true;
  }

  function openDraftModal() {
    if (!draft) return;
    crdYaml = generateCRD(draft);
    showCrdModal = true;
  }

  async function createFromDraft() {
    if (!draft || !draft.name || !draft.html) return;
    creating = true;
    try {
      await patternsApi.create(draft, $auth.token!);
      toasts.success(`Pattern "${draft.name}" created`);
      draft = null; editingName = null;
      await loadPatterns();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to create pattern');
    } finally {
      creating = false;
    }
  }

  async function saveEdit() {
    if (!draft || !editingName || !draft.html) return;
    saving = true;
    try {
      await patternsApi.update(editingName, draft, $auth.token!);
      toasts.success(`Pattern "${editingName}" updated`);
      draft = null; editingName = null;
      await loadPatterns();
    } catch (e: any) {
      toasts.error(e.message || 'Failed to update pattern');
    } finally {
      saving = false;
    }
  }

  async function deletePattern(p: Pattern) {
    if (!confirm(`Delete pattern "${p.name}"? This removes the CRD from the cluster.`)) return;
    try {
      await patternsApi.delete(p.name, $auth.token!);
      patterns = patterns.filter(x => x.name !== p.name || x.scope !== p.scope);
      toasts.success(`Pattern "${p.name}" deleted`);
    } catch (e: any) {
      toasts.error(e.message || 'Failed to delete pattern');
    }
  }

  function copyToClipboard() {
    navigator.clipboard.writeText(crdYaml).then(() => toasts.success('Copied to clipboard'));
  }

  let customPreviewMd = '';
  $: previewTarget = draft ?? null;
  $: defaultPreview = previewTarget?.name
    ? `|||${previewTarget.name}\nHello **world**!\nThis is a _multiline_ block.\n|||\n\nInline: |||${previewTarget.name} inline text|||`
    : '';
  $: previewContent = customPreviewMd || defaultPreview;
</script>

<svelte:head><title>Markdown Patterns — Admin</title></svelte:head>

<div class="p-8 max-w-6xl">
  <div class="flex items-center justify-between mb-6">
    <div>
      <h1 class="text-2xl font-bold text-gray-900">Markdown Patterns</h1>
      <p class="text-sm text-gray-500 mt-1">
        Patterns are managed as <code class="bg-gray-100 px-1 rounded">MarkdownPattern</code> CRDs.
        Use <code class="bg-gray-100 px-1 rounded">kubectl apply</code> to add or remove them.
      </p>
    </div>
    <button class="btn-primary" on:click={startDraft}>+ Generate CRD</button>
  </div>

  {#if draft}
    <!-- ── CRD builder ── -->
    <div class="grid grid-cols-2 gap-6 items-start mb-6">
      <div class="card space-y-4">
        <h2 class="font-semibold text-gray-800">{editingName ? `Edit — ${editingName}` : 'New pattern'}</h2>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label" for="pat-name">Name <span class="text-red-400">*</span></label>
            <input id="pat-name" type="text" class="input font-mono"
              bind:value={draft.name}
              on:input={() => draft && onNameChange(draft.name)}
              placeholder="blink"
              disabled={!!editingName} />
          </div>
          <div>
            <label class="label" for="pat-label">Label</label>
            <input id="pat-label" type="text" class="input" bind:value={draft.label} placeholder="Clignotant" />
          </div>
        </div>

        <div>
          <label class="label" for="pat-scope">Scope</label>
          <input id="pat-scope" type="text" class="input font-mono" bind:value={draft.scope} placeholder="global" />
        </div>

        <div>
          <label class="label" for="pat-html">HTML <span class="text-red-400">*</span></label>
          <p class="text-xs text-gray-400 mb-1"><code class="bg-gray-100 px-1 rounded">&#123;&#123;content&#125;&#125;</code> → rendered markdown.</p>
          <textarea id="pat-html" class="input font-mono text-sm h-20 resize-y" bind:value={draft.html}
            placeholder='<span class="md-pat-blink">&#123;&#123;content&#125;&#125;</span>' />
        </div>

        <div>
          <label class="label" for="pat-css">CSS</label>
          <textarea id="pat-css" class="input font-mono text-sm h-24 resize-y" bind:value={draft.css}
            placeholder=".md-pat-blink &#123; animation: blink 1s step-end infinite; &#125;" />
        </div>

        <div>
          <label class="label" for="pat-js">JS <span class="text-xs text-gray-400 font-normal">(executed after render)</span></label>
          <textarea id="pat-js" class="input font-mono text-sm h-16 resize-y" bind:value={draft.js}
            placeholder="// container is the parent DOM element" />
        </div>

        <div class="flex gap-2 pt-2">
          {#if editingName}
            <button class="btn-primary" on:click={saveEdit}
              disabled={saving || !draft.html}>
              {saving ? 'Saving…' : 'Save changes'}
            </button>
          {:else}
            <button class="btn-primary" on:click={createFromDraft}
              disabled={creating || !draft.name || !draft.html}>
              {creating ? 'Creating…' : 'Create pattern'}
            </button>
          {/if}
          <button class="btn-secondary" on:click={openDraftModal} disabled={!draft.name || !draft.html}>
            ⎈ YAML
          </button>
          <button class="btn-secondary" on:click={() => { draft = null; editingName = null; }}>Cancel</button>
        </div>
      </div>

      <!-- Preview -->
      <div class="card space-y-3 sticky top-4">
        <div class="flex items-center justify-between">
          <h2 class="font-semibold text-gray-800">Preview</h2>
          {#if draft.name}
            <div class="flex gap-2 text-xs text-gray-400">
              <code class="bg-gray-100 px-1 rounded">|||{draft.name}\n...\n|||</code>
              <span>·</span>
              <code class="bg-gray-100 px-1 rounded">|||{draft.name} text|||</code>
            </div>
          {/if}
        </div>
        <div>
          <label class="label text-xs" for="preview-md">Custom markdown</label>
          <textarea id="preview-md" class="input font-mono text-sm h-28 resize-y"
            bind:value={customPreviewMd}
            placeholder="Type markdown here to test…" />
          {#if customPreviewMd}
            <button class="text-xs text-gray-400 hover:text-gray-600 mt-1"
              on:click={() => customPreviewMd = ''}>Reset</button>
          {/if}
        </div>
        <div class="border rounded-lg p-4 bg-gray-50 min-h-[6rem]">
          {#if draft.name && draft.html}
            <Markdown content={previewContent} overridePatterns={{ [draft.name]: draft }} />
          {:else}
            <p class="text-sm text-gray-400 italic">Fill in a name and HTML to preview.</p>
          {/if}
        </div>
      </div>
    </div>
  {/if}

  <!-- ── Pattern list ── -->
  {#if loading}
    <div class="text-gray-400 py-12 text-center">Loading…</div>
  {:else if patterns.length === 0}
    <div class="card text-center py-12 text-gray-400">
      No patterns yet. Apply a <code class="bg-gray-100 px-1 rounded">MarkdownPattern</code> CRD or click <strong>Generate CRD</strong>.
    </div>
  {:else}
    <div class="space-y-2">
      {#each patterns as p (p.name + p.scope)}
        <div class="card flex items-center gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <code class="font-mono font-semibold text-gray-800">|||{p.name}</code>
              {#if p.label}<span class="text-sm text-gray-500">{p.label}</span>{/if}
              <span class="text-xs bg-violet-100 text-violet-700 px-2 py-0.5 rounded-full font-medium">CRD</span>
              {#if p.scope !== 'global'}
                <span class="text-xs bg-yellow-100 text-yellow-700 px-2 py-0.5 rounded-full">{p.scope}</span>
              {:else}
                <span class="text-xs bg-gray-100 text-gray-500 px-2 py-0.5 rounded-full">global</span>
              {/if}
            </div>
            <p class="text-xs text-gray-400 font-mono truncate mt-0.5">{p.html}</p>
          </div>
          <div class="flex gap-2 shrink-0 items-center">
            <button class="text-xs text-violet-600 hover:text-violet-800 font-medium"
              on:click={() => openCrdModal(p)}>⎈ YAML</button>
            <button class="btn-secondary text-xs" on:click={() => startEdit(p)}>Edit</button>
            <button class="text-xs text-red-500 hover:text-red-700"
              on:click={() => deletePattern(p)}>Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- ── CRD YAML modal ── -->
{#if showCrdModal}
  <div class="fixed inset-0 bg-black/40 z-50 flex items-center justify-center p-4"
    on:click|self={() => showCrdModal = false}>
    <div class="bg-white rounded-xl shadow-xl w-full max-w-2xl flex flex-col gap-4 p-6">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold text-gray-800">MarkdownPattern CRD</h2>
        <button class="text-gray-400 hover:text-gray-600 text-xl leading-none"
          on:click={() => showCrdModal = false}>×</button>
      </div>
      <p class="text-xs text-gray-500">
        Apply with <code class="bg-gray-100 px-1 rounded">kubectl apply -f pattern.yaml</code>
      </p>
      <pre class="bg-gray-900 text-green-300 text-xs rounded-lg p-4 overflow-auto max-h-80 font-mono">{crdYaml}</pre>
      <div class="flex gap-2 justify-end">
        <button class="btn-secondary" on:click={copyToClipboard}>Copy</button>
        <button class="btn-primary" on:click={() => showCrdModal = false}>Close</button>
      </div>
    </div>
  </div>
{/if}
