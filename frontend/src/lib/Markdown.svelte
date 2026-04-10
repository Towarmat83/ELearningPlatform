<script lang="ts">
  import { marked } from 'marked';

  export let content: string = '';
  export let inline: boolean = false;

  $: html = inline
    ? (marked.parseInline(content, { async: false }) as string)
    : (marked(content, { async: false }) as string);
</script>

<div class="markdown-content">{@html html}</div>
