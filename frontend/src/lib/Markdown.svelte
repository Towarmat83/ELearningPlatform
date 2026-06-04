<script lang="ts">
  import { onMount, afterUpdate } from 'svelte';
  import { renderMarkdown } from '$lib/markdown';
  import { loadPatterns, patternMap, patterns } from '$lib/patternStore';
  import type { Pattern } from '$lib/api';

  export let content: string = '';
  export let inline: boolean = false;
  export let courseSlug: string | undefined = undefined;
  export let overridePatterns: Record<string, Pattern> = {};

  let container: HTMLDivElement;
  let injectedStyles = new Set<string>();

  onMount(() => loadPatterns(courseSlug));

  $: effectivePatterns = { ...$patternMap, ...overridePatterns };
  $: html = renderMarkdown(content, effectivePatterns, inline);

  function upsertCSS(css: string, name: string, force = false) {
    if (!css) return;
    const existing = document.querySelector<HTMLStyleElement>(`style[data-pattern="${name}"]`);
    if (existing) {
      if (force) existing.textContent = css;
      return;
    }
    if (injectedStyles.has(name)) return;
    const style = document.createElement('style');
    style.setAttribute('data-pattern', name);
    style.textContent = css;
    document.head.appendChild(style);
    injectedStyles.add(name);
  }

  function runJS(js: string) {
    if (!js || !container) return;
    try { new Function('container', js)(container); }
    catch (e) { console.warn('Pattern JS error:', e); }
  }

  afterUpdate(() => {
    // Store patterns — inject once and never update (stable in prod)
    for (const p of $patterns) {
      if (p.css) upsertCSS(p.css, p.name, false);
      if (p.js) runJS(p.js);
    }
    // Override patterns (live preview) — always update style and re-run JS
    for (const p of Object.values(overridePatterns)) {
      if (p.css) upsertCSS(p.css, p.name, true);
      if (p.js) runJS(p.js);
    }
  });
</script>

<div class="markdown-content" bind:this={container}>{@html html}</div>
