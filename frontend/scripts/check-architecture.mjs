import { readdir, readFile } from 'node:fs/promises';
import { resolve, relative, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const root = fileURLToPath(new URL('../src/', import.meta.url));
const layers = ['app', 'pages', 'widgets', 'features', 'entities', 'shared'];
const parts = (path) => path.replaceAll('\\', '/').split('/');
export function boundaryError(from, target) {
  const a = parts(from), b = parts(target);
  if (!layers.includes(a[0]) || !layers.includes(b[0])) return 'Unknown FSD layer';
  const same = a[0] === b[0] && (a[0] === 'app' || a[1] === b[1]);
  if (same) return null;
  if (a[0] !== 'shared' || b[0] !== 'shared') {
    if (layers.indexOf(b[0]) <= layers.indexOf(a[0])) return 'Upward or cross-slice import';
  }
  if (b.length > 2 && !/^index(?:\.ts)?$/.test(b.slice(2).join('/'))) return 'Import bypasses public index.ts';
  return null;
}
async function files(path) {
  const entries = await readdir(path, { withFileTypes: true });
  return (await Promise.all(entries.map((e) => e.isDirectory() ? files(resolve(path, e.name)) : [resolve(path, e.name)]))).flat();
}
async function check() {
  const errors = [];
  for (const path of await files(root)) {
    if (!/\.tsx?$/.test(path)) continue;
    const from = relative(root, path).replaceAll('\\', '/');
    const source = ts.createSourceFile(path, await readFile(path, 'utf8'), ts.ScriptTarget.Latest, true);
    function visit(node) {
      let target;
      if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier) target = node.moduleSpecifier;
      if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument)) target = node.argument.literal;
      if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword) target = node.arguments[0];
      if (target) {
        if (!ts.isStringLiteral(target)) errors.push(`${from}: computed import is forbidden`);
        else if (target.text.startsWith('@/') || target.text.startsWith('.')) {
          const to = target.text.startsWith('@/') ? target.text.slice(2) : relative(root, resolve(dirname(path), target.text));
          const error = boundaryError(from, to);
          if (error) errors.push(`${from} -> ${target.text}: ${error}`);
        }
      }
      if (!from.includes('/api/') && (ts.isCallExpression(node) || ts.isNewExpression(node))) {
        const name = node.expression.getText(source);
        if (/^(?:.*\.)?(?:fetch|XMLHttpRequest)$/.test(name) || /^axios(?:\.|$)/.test(name)) errors.push(`${from}: network call outside api segment`);
      }
      ts.forEachChild(node, visit);
    }
    visit(source);
  }
  if (errors.length) throw new Error(errors.join('\n'));
  console.log('PASS: FSD layer/public API boundaries and network placement');
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await check();
