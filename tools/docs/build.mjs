// Builds the static documentation site from docs/*.md.
//
// Deliberately not Jekyll: GitHub Pages' default Jekyll does not render mermaid,
// so a custom layout is needed either way — and a Node build can be run and
// checked locally, which a Ruby one cannot be on most of our machines.
//
//   node build.mjs [--out <dir>]
//
// Output is a self-contained directory of HTML. Mermaid renders client-side from
// a CDN; everything else is inlined.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Marked } from 'marked';

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
const docsDir = path.join(repo, 'docs');

const outFlag = process.argv.indexOf('--out');
const outDir = outFlag >= 0 ? path.resolve(process.argv[outFlag + 1]) : path.join(repo, 'site');

// Order matters: this is the reading order and the nav order.
const PAGES = [
  { file: 'index.md', title: 'Overview', blurb: 'Start here' },
  { file: 'ARCHITECTURE.md', title: 'Architecture', blurb: 'Layers, request lifecycle, exit codes' },
  { file: 'SCHEDULING.md', title: 'Scheduling', blurb: 'Itinerary editing and the json0 op engine' },
  { file: 'EXTENDING.md', title: 'Extending', blurb: 'Adding a command, with a worked example' },
  { file: 'API.md', title: 'HTTP API', blurb: "Wanderlog's private API, endpoint by endpoint" },
  { file: 'AUTH.md', title: 'Auth', blurb: 'The session scheme and how it was found' },
];

const marked = new Marked({ gfm: true, breaks: false });

// Mermaid fences must survive as <pre class="mermaid"> for the client-side
// renderer to find them. Default highlighting would escape and wrap them.
marked.use({
  renderer: {
    code({ text, lang }) {
      if (lang === 'mermaid') {
        return `<pre class="mermaid">${escapeHtml(text)}</pre>\n`;
      }
      const cls = lang ? ` class="language-${escapeHtml(lang)}"` : '';
      return `<pre><code${cls}>${escapeHtml(text)}</code></pre>\n`;
    },
    // Cross-document links are written as *.md so they work on GitHub too.
    link({ href, title, tokens }) {
      const text = this.parser.parseInline(tokens);
      let h = href || '';
      if (/^[^/]+\.md(#.*)?$/i.test(h)) h = h.replace(/\.md(?=$|#)/i, '.html');
      const t = title ? ` title="${escapeHtml(title)}"` : '';
      const ext = /^https?:/i.test(h) ? ' target="_blank" rel="noopener"' : '';
      return `<a href="${escapeHtml(h)}"${t}${ext}>${text}</a>`;
    },
  },
});

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

const CSS = `
:root {
  color-scheme: light dark;
  --bg: #ffffff; --fg: #1f2328; --muted: #59636e;
  --line: #d1d9e0; --accent: #0969da; --code-bg: #f6f8fa;
  --side-bg: #f6f8fa; --note-bg: #f6f8fa;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #9198a1;
    --line: #3d444d; --accent: #4493f8; --code-bg: #151b23;
    --side-bg: #151b23; --note-bg: #151b23;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--fg);
  font: 16px/1.65 -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
}
.wrap { display: flex; align-items: flex-start; max-width: 1280px; margin: 0 auto; }
nav {
  position: sticky; top: 0; flex: 0 0 260px; height: 100vh; overflow-y: auto;
  padding: 28px 20px; background: var(--side-bg); border-right: 1px solid var(--line);
}
nav .brand { font-weight: 700; font-size: 18px; letter-spacing: -0.01em; }
nav .brand a { color: var(--fg); text-decoration: none; }
nav .tag { color: var(--muted); font-size: 13px; margin: 4px 0 22px; }
nav ol { list-style: none; margin: 0; padding: 0; }
nav li { margin-bottom: 14px; }
nav a.item { color: var(--accent); text-decoration: none; font-weight: 600; font-size: 14.5px; }
nav a.item:hover { text-decoration: underline; }
nav a.item.active { color: var(--fg); }
nav .blurb { color: var(--muted); font-size: 12.5px; line-height: 1.45; margin-top: 2px; }
nav .repo { margin-top: 26px; padding-top: 18px; border-top: 1px solid var(--line); font-size: 13px; }
main { flex: 1 1 auto; min-width: 0; padding: 40px 48px 96px; }
main > :first-child { margin-top: 0; }
h1, h2, h3 { line-height: 1.25; letter-spacing: -0.015em; }
h1 { font-size: 30px; padding-bottom: 8px; border-bottom: 1px solid var(--line); }
h2 { font-size: 22px; margin-top: 38px; padding-bottom: 6px; border-bottom: 1px solid var(--line); }
h3 { font-size: 17px; margin-top: 26px; }
a { color: var(--accent); }
code {
  background: var(--code-bg); padding: 0.15em 0.4em; border-radius: 6px; font-size: 85%;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
}
pre {
  background: var(--code-bg); padding: 14px 16px; border-radius: 8px; overflow-x: auto;
  border: 1px solid var(--line);
}
pre code { background: none; padding: 0; font-size: 13.5px; }
table { border-collapse: collapse; width: 100%; margin: 18px 0; display: block; overflow-x: auto; }
th, td { border: 1px solid var(--line); padding: 7px 13px; text-align: left; vertical-align: top; }
th { background: var(--code-bg); font-weight: 600; }
blockquote { margin: 0; padding: 0 1em; color: var(--muted); border-left: 3px solid var(--line); }
hr { border: 0; border-top: 1px solid var(--line); margin: 34px 0; }
details {
  margin: 20px 0; padding: 14px 18px; background: var(--note-bg);
  border: 1px solid var(--line); border-radius: 8px;
}
details > summary {
  cursor: pointer; font-weight: 600; margin: -14px -18px; padding: 14px 18px;
}
details[open] > summary { margin-bottom: 4px; border-bottom: 1px solid var(--line); }
details > summary::marker { color: var(--muted); }
pre.mermaid {
  background: transparent; border: 0; text-align: center; padding: 8px 0;
  /* Hidden until mermaid has drawn it, so raw source never flashes. */
  visibility: hidden;
}
pre.mermaid[data-processed="true"] { visibility: visible; }
.mermaid-failed {
  visibility: visible; color: var(--muted); font-size: 13px;
  border: 1px dashed var(--line); border-radius: 8px; text-align: left; padding: 12px 14px;
}
@media (max-width: 900px) {
  .wrap { flex-direction: column; }
  nav { position: static; flex: none; width: 100%; height: auto; border-right: 0; border-bottom: 1px solid var(--line); }
  main { padding: 28px 20px 64px; }
}
`;

function nav(currentFile) {
  const items = PAGES.map(p => {
    const href = p.file === 'index.md' ? 'index.html' : p.file.replace(/\.md$/, '.html');
    const active = p.file === currentFile ? ' active' : '';
    return `      <li>
        <a class="item${active}" href="${href}">${escapeHtml(p.title)}</a>
        <div class="blurb">${escapeHtml(p.blurb)}</div>
      </li>`;
  }).join('\n');

  return `  <nav>
    <div class="brand"><a href="index.html">wlog</a></div>
    <div class="tag">Wanderlog CLI for AI agents</div>
    <ol>
${items}
    </ol>
    <div class="repo"><a href="https://github.com/KRamdath/wanderlog-cli" target="_blank" rel="noopener">GitHub repository</a></div>
  </nav>`;
}

function layout({ title, bodyHtml, currentFile }) {
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escapeHtml(title)} — wlog docs</title>
<style>${CSS}</style>
</head>
<body>
<div class="wrap">
${nav(currentFile)}
  <main>
${bodyHtml}
  </main>
</div>
<script type="module">
  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
  const dark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  mermaid.initialize({ startOnLoad: false, theme: dark ? 'dark' : 'default' });
  // Render one at a time so a single bad diagram cannot blank the whole page.
  for (const el of document.querySelectorAll('pre.mermaid')) {
    try {
      await mermaid.run({ nodes: [el] });
    } catch (e) {
      el.classList.add('mermaid-failed');
      el.textContent = 'Diagram failed to render: ' + (e && e.message ? e.message : e);
    }
  }
</script>
</body>
</html>
`;
}

// --- build ---------------------------------------------------------------

fs.rmSync(outDir, { recursive: true, force: true });
fs.mkdirSync(outDir, { recursive: true });

let pageCount = 0;
let mermaidCount = 0;
let detailsCount = 0;

for (const page of PAGES) {
  const src = path.join(docsDir, page.file);
  if (!fs.existsSync(src)) {
    console.error(`missing: docs/${page.file}`);
    process.exitCode = 1;
    continue;
  }

  const md = fs.readFileSync(src, 'utf8').split(String.fromCharCode(13)).join('');
  const bodyHtml = marked.parse(md);

  const html = layout({ title: page.title, bodyHtml, currentFile: page.file });
  const outName = page.file === 'index.md' ? 'index.html' : page.file.replace(/\.md$/, '.html');
  fs.writeFileSync(path.join(outDir, outName), html, 'utf8');

  const m = (bodyHtml.match(/<pre class="mermaid">/g) || []).length;
  const d = (bodyHtml.match(/<details>/g) || []).length;
  mermaidCount += m;
  detailsCount += d;
  pageCount++;
  console.log(`  ${outName.padEnd(20)} ${String(m).padStart(2)} diagrams  ${String(d).padStart(2)} advanced sections`);
}

// Tells GitHub Pages not to run the output through Jekyll.
fs.writeFileSync(path.join(outDir, '.nojekyll'), '', 'utf8');

console.log(`\n${pageCount} pages, ${mermaidCount} diagrams, ${detailsCount} advanced sections → ${outDir}`);
