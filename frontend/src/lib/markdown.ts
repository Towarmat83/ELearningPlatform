import { marked } from 'marked';
import hljs from 'highlight.js';

// Configure marked once with syntax highlighting
marked.use({
  renderer: {
    code({ text, lang }: { text: string; lang?: string }) {
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext';
      const highlighted = hljs.highlight(text, { language }).value;
      return `<pre class="hljs-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`;
    },
  },
});

export { marked };
