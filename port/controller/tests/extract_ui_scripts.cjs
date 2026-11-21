#!/usr/bin/env node
'use strict';

// Extract executable JavaScript rather than linting HTML/CSS as JavaScript.
// The two UIs are checked independently; the smoke suite uses the same R11 UI.
const fs = require('node:fs');
const path = require('node:path');

const controllerDir = path.resolve(__dirname, '..');
const generatedDir = path.join(__dirname, '.generated');
fs.mkdirSync(generatedDir, { recursive: true });
for (const [input, name] of [
  ['r11_original.html', 'r11_original.js'],
  ['frontend.html', 'frontend.js'],
]) {
  const html = fs.readFileSync(path.join(controllerDir, input), 'utf8');
  const scripts = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi)];
  if (!scripts.length) throw new Error(input + ': no executable script');
  const content = scripts.map(x => x[1]).join('\n;\n');
  fs.writeFileSync(path.join(generatedDir, name), content);
  console.log(input + ': ' + scripts.length + ' script blocks, ' + content.length + ' JS characters');
}
