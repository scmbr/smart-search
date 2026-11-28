import { readFile } from 'node:fs/promises';
import openapiTS, { astToString } from 'openapi-typescript';

const schema = new URL('../../docs/openapi.yaml', import.meta.url);
const target = new URL('../src/shared/api/generated/schema.ts', import.meta.url);
const expected = astToString(await openapiTS(schema));
const actual = await readFile(target, 'utf8');
// CLI adds a standard generated-file header; compare the generated declarations.
const declarations = (text) => text.slice(text.indexOf('export interface paths'));
if (declarations(actual) !== declarations(expected)) {
  throw new Error('OpenAPI types differ. Run npm run generate:api.');
}
console.log('PASS: API types match docs/openapi.yaml');
