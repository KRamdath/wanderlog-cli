// Parses every mermaid block in docs/*.md and fails if any of them is invalid.
//
// A broken diagram renders as a grey error box on GitHub and as a failure
// message on the docs site, neither of which is loud enough to notice. This
// catches it in CI instead.
//
//   node validate.mjs ../../docs
//
// Two traps, both of which have already bitten:
//   - A semicolon inside a sequenceDiagram note is a statement separator.
//   - CRLF files yield zero blocks if you match fences with a naive regex, so
//     the check silently passes. Carriage returns are stripped first and the
//     fences are matched line by line.
//
// mermaid pulls in DOMPurify, which needs a DOM even for parse-only, hence jsdom.

import fs from 'node:fs';
import path from 'node:path';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!DOCTYPE html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.DOMPurify = dom.window.DOMPurify;

const mermaid = (await import('mermaid')).default;
mermaid.initialize({ startOnLoad: false, securityLevel: 'loose' });

const CR = String.fromCharCode(13);
const FENCE = '```mermaid';
const CLOSE = '```';

const dir = process.argv[2];
const files = fs.readdirSync(dir).filter(f => f.endsWith('.md')).sort();
let fail = 0, total = 0;

for (const name of files) {
  const src = fs.readFileSync(path.join(dir, name), 'utf8').split(CR).join('');
  const blocks = [];
  let cur = null;
  for (const line of src.split('\n')) {
    if (cur === null && line.trim() === FENCE) { cur = []; continue; }
    if (cur !== null && line.trim() === CLOSE) { blocks.push(cur.join('\n')); cur = null; continue; }
    if (cur !== null) cur.push(line);
  }
  if (cur !== null) { console.log('  WARN ' + name + ': unterminated mermaid fence'); fail++; }

  for (let i = 0; i < blocks.length; i++) {
    total++;
    const kind = blocks[i].trim().split('\n')[0].trim();
    try {
      await mermaid.parse(blocks[i]);
      console.log('  OK   ' + name + ' #' + (i + 1) + ' (' + kind + ')');
    } catch (e) {
      fail++;
      console.log('  FAIL ' + name + ' #' + (i + 1) + ' (' + kind + ')');
      console.log('       ' + String(e.message || e).split('\n').slice(0, 4).join('\n       '));
    }
  }
}
console.log('');
console.log((total - fail) + '/' + total + ' diagrams parse across ' + files.length + ' files');
process.exit(fail ? 1 : 0);
