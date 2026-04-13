// ─────────────────────────────────────────────────────────────────────────────
// Labs as Code — JSON standard, Markdown parser, validation
// ─────────────────────────────────────────────────────────────────────────────

// ─── Types ───────────────────────────────────────────────────────────────────

export interface LabExport {
  version: '1.0';
  type: 'form' | 'ctf' | 'interactive';
  title: string;
  description: string;
  points: number;
  order_index: number;
  is_published: boolean;
  /** CTF single: "FLAG{...}" | CTF multi: JSON-encoded map { flag_id: "FLAG{...}" } */
  flag?: string;
  content: LabExportContent;
}

export interface LabExportContent {
  // ── form ──────────────────────────────────────────────────────────────────
  questions?: QuestionExport[];
  // ── ctf single ────────────────────────────────────────────────────────────
  challenge?: string;
  category?: string;
  hints?: string[];
  resources?: ResourceExport[];
  // ── ctf multi ─────────────────────────────────────────────────────────────
  flags?: CtfFlagExport[];
  instructions?: string;
  // ── shared optional ───────────────────────────────────────────────────────
  docker_image?: string;
  // ── interactive ───────────────────────────────────────────────────────────
  steps?: StepExport[];
}

export interface QuestionExport {
  id: string;
  text: string;
  type: 'multiple_choice' | 'text';
  options?: string[];
  correct_answer: string;
  points: number;
  explanation?: string;
}

export interface CtfFlagExport {
  id: string;
  name: string;
  description: string;
  points: number;
  /** Secret flag value — present in exports, stripped from student API */
  flag: string;
}

export interface ResourceExport {
  name: string;
  url: string;
}

export interface StepExport {
  id: string;
  title: string;
  description: string;
  commands?: CommandExport[];
}

export interface CommandExport {
  cmd: string;
  explanation?: string;
}

// ─── Validation ──────────────────────────────────────────────────────────────

/**
 * Validates a parsed object against the LabExport schema.
 * Throws a descriptive Error on failure, returns typed LabExport on success.
 */
export function validateLabJSON(raw: unknown): LabExport {
  if (!raw || typeof raw !== 'object') throw new Error('Invalid JSON: root must be an object');
  const lab = raw as Record<string, unknown>;

  if (lab.version !== '1.0') throw new Error('Missing or invalid "version": must be "1.0"');
  if (!['form', 'ctf', 'interactive'].includes(lab.type as string)) {
    throw new Error('Missing or invalid "type": must be "form", "ctf", or "interactive"');
  }
  if (typeof lab.title !== 'string' || !lab.title.trim()) {
    throw new Error('Missing or empty "title"');
  }
  if (typeof lab.description !== 'string') {
    throw new Error('Missing "description" (can be empty string)');
  }
  if (typeof lab.points !== 'number') throw new Error('"points" must be a number');
  if (!lab.content || typeof lab.content !== 'object') {
    throw new Error('Missing "content" object');
  }

  const content = lab.content as Record<string, unknown>;
  const type = lab.type as string;

  if (type === 'form') {
    if (!Array.isArray(content.questions) || content.questions.length === 0) {
      throw new Error('"content.questions" must be a non-empty array for form labs');
    }
    for (const [i, q] of (content.questions as unknown[]).entries()) {
      if (!q || typeof q !== 'object') throw new Error(`Question ${i + 1}: must be an object`);
      const qo = q as Record<string, unknown>;
      if (typeof qo.text !== 'string' || !qo.text.trim()) {
        throw new Error(`Question ${i + 1}: missing "text"`);
      }
      if (!['multiple_choice', 'text'].includes(qo.type as string)) {
        throw new Error(`Question ${i + 1}: "type" must be "multiple_choice" or "text"`);
      }
      if (typeof qo.correct_answer !== 'string') {
        throw new Error(`Question ${i + 1}: missing "correct_answer"`);
      }
    }
  } else if (type === 'interactive') {
    if (typeof content.docker_image !== 'string' || !content.docker_image.trim()) {
      throw new Error('"content.docker_image" is required for interactive labs');
    }
    if (!Array.isArray(content.steps) || content.steps.length === 0) {
      throw new Error('"content.steps" must be a non-empty array for interactive labs');
    }
  } else if (type === 'ctf') {
    const flags = content.flags as unknown[] | undefined;
    if (flags && flags.length > 0) {
      // multi-flag
    } else {
      if (typeof lab.flag !== 'string' || !lab.flag.trim()) {
        throw new Error('"flag" is required for single-flag CTF labs');
      }
    }
  }

  return lab as unknown as LabExport;
}

// ─── Markdown Parser ─────────────────────────────────────────────────────────

/**
 * Parses a Markdown document into a LabExport.
 * Throws with a descriptive message on failure.
 */
export function parseMarkdownToLab(markdown: string): LabExport {
  const { meta, body } = parseFrontmatter(markdown);

  if (!meta.title) throw new Error('Missing required frontmatter: title');
  if (!meta.type) throw new Error('Missing required frontmatter: type (form | ctf | interactive)');

  const type = meta.type as 'form' | 'ctf' | 'interactive';
  if (!['form', 'ctf', 'interactive'].includes(type)) {
    throw new Error(`Invalid type "${meta.type}" — must be: form, ctf, or interactive`);
  }

  const points = meta.points ? parseInt(meta.points) : 100;
  // Description: use frontmatter value or first paragraph before any ## section
  const introText = body.split(/^## /m)[0].trim();
  const description = meta.description ?? introText;

  const base = {
    version: '1.0' as const,
    type,
    title: meta.title,
    description,
    points: isNaN(points) ? 100 : points,
    order_index: meta.order_index ? (parseInt(meta.order_index) || 0) : 0,
    is_published: meta.is_published === 'true',
  };

  if (type === 'interactive') {
    if (!meta.docker_image) {
      throw new Error('Missing required frontmatter for interactive labs: docker_image');
    }
    const steps = parseInteractiveSteps(body);
    if (steps.length === 0) {
      throw new Error('Interactive lab must have at least one ## Step section');
    }
    return { ...base, content: { docker_image: meta.docker_image, steps } };
  }

  if (type === 'form') {
    const questions = parseFormQuestions(body);
    if (questions.length === 0) {
      throw new Error('Form lab must have at least one ## Question section');
    }
    return { ...base, content: { questions } };
  }

  // CTF
  const isMulti = meta.mode === 'multi';
  const ctfData = parseCtfBody(body, isMulti);

  if (isMulti) {
    const flagMap: Record<string, string> = {};
    for (const f of ctfData.flags ?? []) flagMap[f.id] = f.flag;
    return {
      ...base,
      flag: JSON.stringify(flagMap),
      content: {
        flags: ctfData.flags?.map(({ flag: _ignored, ...rest }) => ({ ...rest, flag: _ignored })),
        instructions: ctfData.instructions,
        ...(meta.docker_image ? { docker_image: meta.docker_image } : {}),
        ...(ctfData.hints.length ? { hints: ctfData.hints } : {}),
      },
    };
  } else {
    if (!meta.flag) throw new Error('Missing required frontmatter for CTF labs: flag');
    return {
      ...base,
      flag: meta.flag,
      content: {
        challenge: ctfData.challenge,
        category: meta.category ?? 'misc',
        ...(ctfData.hints.length ? { hints: ctfData.hints } : {}),
        ...(ctfData.resources.length ? { resources: ctfData.resources } : {}),
        ...(meta.docker_image ? { docker_image: meta.docker_image } : {}),
      },
    };
  }
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

function parseFrontmatter(markdown: string): { meta: Record<string, string>; body: string } {
  const lines = markdown.trimStart().split('\n');
  const meta: Record<string, string> = {};
  let bodyStart = 0;

  if (lines[0]?.trim() === '---') {
    let i = 1;
    while (i < lines.length && lines[i].trim() !== '---') {
      const colonIdx = lines[i].indexOf(':');
      if (colonIdx > 0) {
        const key = lines[i].slice(0, colonIdx).trim();
        const val = lines[i].slice(colonIdx + 1).trim();
        if (key) meta[key] = val;
      }
      i++;
    }
    bodyStart = i + 1;
  }

  return { meta, body: lines.slice(bodyStart).join('\n').trim() };
}

function splitH2Sections(body: string): Array<{ title: string; body: string }> {
  const sections: Array<{ title: string; body: string }> = [];
  const parts = body.split(/^## /m);
  for (const part of parts) {
    const trimmed = part.trim();
    if (!trimmed) continue;
    const nlIdx = trimmed.indexOf('\n');
    if (nlIdx === -1) {
      sections.push({ title: trimmed, body: '' });
    } else {
      sections.push({ title: trimmed.slice(0, nlIdx).trim(), body: trimmed.slice(nlIdx + 1).trim() });
    }
  }
  return sections;
}

function extractBashCommands(text: string): CommandExport[] {
  const cmds: CommandExport[] = [];
  const blockRegex = /```(?:bash|sh|shell)?\n([\s\S]*?)```/g;
  let match: RegExpExecArray | null;
  while ((match = blockRegex.exec(text)) !== null) {
    for (const line of match[1].split('\n')) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;
      const hashIdx = trimmed.indexOf(' #');
      if (hashIdx > 0) {
        cmds.push({ cmd: trimmed.slice(0, hashIdx).trim(), explanation: trimmed.slice(hashIdx + 2).trim() || undefined });
      } else {
        cmds.push({ cmd: trimmed });
      }
    }
  }
  return cmds;
}

function parseInteractiveSteps(body: string): StepExport[] {
  const sections = splitH2Sections(body);
  return sections.map((sec, i) => {
    const commands = extractBashCommands(sec.body);
    const description = sec.body.replace(/```[\s\S]*?```/g, '').trim();
    return { id: `step${i + 1}`, title: sec.title, description, commands };
  });
}

function parseFormQuestions(body: string): QuestionExport[] {
  const sections = splitH2Sections(body);
  return sections.map((sec, i) => {
    const lines = sec.body.split('\n');
    const isText = sec.body.includes('<!-- type: text -->');

    if (isText) {
      let answer = '';
      const explanationLines: string[] = [];
      for (const line of lines) {
        const ansMatch = line.match(/^\*\*Answer:\*\*\s*(.+)$/);
        if (ansMatch) answer = ansMatch[1].trim();
        const expMatch = line.match(/^>\s*(.+)$/);
        if (expMatch) explanationLines.push(expMatch[1]);
      }
      return {
        id: `q${i + 1}`,
        text: sec.title,
        type: 'text' as const,
        correct_answer: answer,
        points: 10,
        ...(explanationLines.length ? { explanation: explanationLines.join(' ') } : {}),
      };
    }

    // Multiple choice
    const options: string[] = [];
    let correct_answer = '';
    const explanationLines: string[] = [];
    for (const line of lines) {
      const correct = line.match(/^- \[x\]\s*(.+)$/i);
      const wrong = line.match(/^- \[ \]\s*(.+)$/);
      const exp = line.match(/^>\s*(.+)$/);
      if (correct) { correct_answer = correct[1].trim(); options.push(correct_answer); }
      else if (wrong) { options.push(wrong[1].trim()); }
      else if (exp) { explanationLines.push(exp[1]); }
    }

    return {
      id: `q${i + 1}`,
      text: sec.title,
      type: 'multiple_choice' as const,
      ...(options.length ? { options } : {}),
      correct_answer,
      points: 10,
      ...(explanationLines.length ? { explanation: explanationLines.join(' ') } : {}),
    };
  });
}

function parseCtfBody(body: string, isMulti: boolean): {
  challenge?: string;
  hints: string[];
  resources: ResourceExport[];
  flags?: CtfFlagExport[];
  instructions?: string;
} {
  const sections = splitH2Sections(body);
  const hintsSection = sections.find(s => s.title.toLowerCase() === 'hints');
  const resourcesSection = sections.find(s => s.title.toLowerCase() === 'resources');
  const flagsSection = sections.find(s => s.title.toLowerCase() === 'flags');
  const otherSections = sections.filter(
    s => !['hints', 'resources', 'flags'].includes(s.title.toLowerCase())
  );

  const hints = hintsSection
    ? hintsSection.body.split('\n').map(l => l.replace(/^[-*]\s*/, '').trim()).filter(Boolean)
    : [];

  const resources: ResourceExport[] = [];
  if (resourcesSection) {
    for (const line of resourcesSection.body.split('\n')) {
      const m = line.match(/\[([^\]]+)\]\(([^)]+)\)/);
      if (m) resources.push({ name: m[1], url: m[2] });
    }
  }

  if (isMulti && flagsSection) {
    const flags: CtfFlagExport[] = [];
    const parts = flagsSection.body.split(/^### /m).filter(p => p.trim());
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const nlIdx = part.indexOf('\n');
      const header = nlIdx > -1 ? part.slice(0, nlIdx) : part;
      const flagBody = nlIdx > -1 ? part.slice(nlIdx + 1).trim() : '';
      const ptsMatch = header.match(/^(.+?)\s*\((\d+)\s*pts?\)/i);
      const name = ptsMatch ? ptsMatch[1].trim() : header.trim();
      const points = ptsMatch ? parseInt(ptsMatch[2]) : 50;
      const flagValueMatch = flagBody.match(/^flag:\s*(.+)$/im);
      const flagValue = flagValueMatch ? flagValueMatch[1].trim() : '';
      const description = flagBody.replace(/^flag:\s*.+$/im, '').trim();
      flags.push({ id: `flag${i + 1}`, name, description, points, flag: flagValue });
    }
    const introText = body.split(/^## /m)[0].trim();
    const extraSections = otherSections.map(s => `## ${s.title}\n${s.body}`).join('\n\n');
    const instructions = [introText, extraSections].filter(Boolean).join('\n\n');
    return { hints, resources, flags, instructions };
  }

  const introText = body.split(/^## /m)[0].trim();
  const extraParts = otherSections.map(s => `## ${s.title}\n${s.body}`);
  const challenge = [introText, ...extraParts].filter(Boolean).join('\n\n');
  return { challenge, hints, resources };
}

// ─── Templates ───────────────────────────────────────────────────────────────

export const INTERACTIVE_JSON_TEMPLATE = JSON.stringify({
  version: '1.0',
  type: 'interactive',
  title: 'Linux Basics',
  description: 'Learn fundamental Linux navigation commands.',
  points: 100,
  order_index: 0,
  is_published: false,
  content: {
    docker_image: 'ubuntu:22.04',
    steps: [
      {
        id: 'step1',
        title: 'Explore the Filesystem',
        description: 'Use `pwd` and `ls` to navigate the filesystem.',
        commands: [
          { cmd: 'pwd', explanation: 'Print working directory' },
          { cmd: 'ls -la', explanation: 'List all files with details' },
        ],
      },
      {
        id: 'step2',
        title: 'Create Files',
        description: 'Create and read files with `touch`, `echo`, and `cat`.',
        commands: [
          { cmd: 'touch hello.txt', explanation: 'Create an empty file' },
          { cmd: 'echo "Hello!" > hello.txt', explanation: 'Write text to file' },
          { cmd: 'cat hello.txt', explanation: 'Display file content' },
        ],
      },
    ],
  },
}, null, 2);

export const FORM_JSON_TEMPLATE = JSON.stringify({
  version: '1.0',
  type: 'form',
  title: 'Linux Commands Quiz',
  description: 'Test your knowledge of basic Linux commands.',
  points: 100,
  order_index: 0,
  is_published: false,
  content: {
    questions: [
      {
        id: 'q1',
        text: 'Which command lists the contents of a directory?',
        type: 'multiple_choice',
        options: ['ls', 'dir', 'list', 'show'],
        correct_answer: 'ls',
        points: 25,
        explanation: '`ls` is the standard Unix command for listing directory contents.',
      },
      {
        id: 'q2',
        text: 'What does the `pwd` command stand for?',
        type: 'text',
        correct_answer: 'print working directory',
        points: 25,
        explanation: 'pwd = "print working directory" — shows your current location.',
      },
    ],
  },
}, null, 2);

export const CTF_JSON_TEMPLATE = JSON.stringify({
  version: '1.0',
  type: 'ctf',
  title: 'SQL Injection Challenge',
  description: 'Find and exploit the SQL injection vulnerability to retrieve the flag.',
  points: 200,
  order_index: 0,
  is_published: false,
  flag: 'FLAG{sql_injection_master}',
  content: {
    challenge: 'The login form is vulnerable to SQL injection.\n\nFind a way to bypass authentication and retrieve the hidden flag.',
    category: 'web',
    hints: [
      "Try a single quote `'` in the username field",
      'What happens when a WHERE clause always evaluates to true?',
    ],
    resources: [
      { name: 'SQL Injection Cheat Sheet', url: 'https://portswigger.net/web-security/sql-injection/cheat-sheet' },
    ],
  },
}, null, 2);

export const INTERACTIVE_MD_TEMPLATE = `---
title: Linux Navigation
type: interactive
docker_image: ubuntu:22.04
points: 100
description: Learn fundamental Linux navigation commands.
---

## Explore the Filesystem

Use \`pwd\` and \`ls\` to understand where you are and what files exist.

\`\`\`bash
pwd # Print working directory
ls -la # List all files with details
cd /tmp # Navigate to /tmp
\`\`\`

## Create and Manipulate Files

Create files, write content, and read them back.

\`\`\`bash
touch hello.txt # Create an empty file
echo "Hello World" > hello.txt # Write text to the file
cat hello.txt # Display file content
\`\`\`
`;

export const FORM_MD_TEMPLATE = `---
title: Linux Commands Quiz
type: form
points: 100
description: Test your knowledge of basic Linux commands.
---

## Which command lists directory contents?

- [x] ls
- [ ] dir
- [ ] list
- [ ] show

> \`ls\` is the standard Unix command for listing directory contents.

## What does the pwd command stand for?
<!-- type: text -->

**Answer:** print working directory

> pwd = "print working directory" — shows your current location in the filesystem.
`;

export const CTF_MD_TEMPLATE = `---
title: SQL Injection Challenge
type: ctf
category: web
flag: FLAG{sql_injection_master}
points: 200
description: Find and exploit the SQL injection vulnerability.
---

The login form is vulnerable to SQL injection. Bypass authentication and retrieve the flag.

## Hints

- Try a single quote \`'\` in the username field
- What happens when a WHERE clause always evaluates to true?

## Resources

- [SQL Injection Cheat Sheet](https://portswigger.net/web-security/sql-injection/cheat-sheet)
`;

export const CTF_MULTI_MD_TEMPLATE = `---
title: Linux Privilege Escalation
type: ctf
mode: multi
docker_image: ubuntu:22.04
points: 300
description: Escalate from a low-privilege user to root.
---

You have access to a low-privilege shell. Find all three flags.

## Flags

### User Flag (100 pts)
Find user.txt in the home directory.
flag: FLAG{user_flag_here}

### Service Flag (100 pts)
A running service exposes a second flag.
flag: FLAG{service_flag_here}

### Root Flag (100 pts)
Escalate to root and find root.txt.
flag: FLAG{root_flag_here}

## Hints

- Check SUID binaries: \`find / -perm -4000 2>/dev/null\`
- Look for writable cron scripts
`;
