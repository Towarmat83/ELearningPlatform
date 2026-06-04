import { marked } from 'marked';
import hljs from 'highlight.js';
import type { Pattern } from '$lib/api';

// ── Syntax highlighting ────────────────────────────────────────────────────

marked.use({
  renderer: {
    code({ text, lang }: { text: string; lang?: string }) {
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext';
      const highlighted = hljs.highlight(text, { language }).value;
      return `<pre class="hljs-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`;
    },
  },
});

// ── Pattern map ───────────────────────────────────────────────────────────

let _patternMap: Record<string, Pattern> = {};

export function setPatterns(map: Record<string, Pattern>) {
  _patternMap = map;
}

function applyPattern(name: string, innerHtml: string): string {
  const p = _patternMap[name];
  if (!p) return innerHtml;
  return p.html.replace('{{content}}', innerHtml);
}

// ── Block extension: |||name\n...content...\n||| ───────────────────────────
//
// start() must return the earliest index where this token COULD begin.
// Using indexOf('|||') covers both position-0 and mid-document occurrences.

const blockPattern: marked.TokenizerExtension & marked.RendererExtension = {
  name: 'patternBlock',
  level: 'block',
  start(src: string) {
    return src.indexOf('|||');
  },
  tokenizer(src: string) {
    const match = src.match(/^\|\|\|(\S+)\n([\s\S]*?)\n\|\|\|[ \t]*(?:\n|$)/);
    if (match) {
      return {
        type: 'patternBlock',
        raw: match[0],
        patternName: match[1],
        content: match[2],
      };
    }
  },
  renderer(token: marked.Tokens.Generic) {
    const inner = marked.parse(token.content, { async: false }) as string;
    return applyPattern(token.patternName, inner);
  },
};

// ── Inline extension: |||name|content||| ─────────────────────────────────

const inlinePattern: marked.TokenizerExtension & marked.RendererExtension = {
  name: 'patternInline',
  level: 'inline',
  start(src: string) {
    return src.indexOf('|||');
  },
  tokenizer(src: string) {
    // |||name text content|||  — name is the first word, rest is content
    const match = src.match(/^\|\|\|(\S+) ([^|]+?)\|\|\|/);
    if (match) {
      return {
        type: 'patternInline',
        raw: match[0],
        patternName: match[1],
        content: match[2],
      };
    }
  },
  renderer(token: marked.Tokens.Generic) {
    return applyPattern(token.patternName, token.content);
  },
};

marked.use({ extensions: [blockPattern, inlinePattern] });

export { marked };

// Render with an explicit pattern map so callers can force re-evaluation.
export function renderMarkdown(content: string, patterns: Record<string, Pattern>, inline = false): string {
  setPatterns(patterns);
  return inline
    ? (marked.parseInline(content, { async: false }) as string)
    : (marked(content, { async: false }) as string);
}
