import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { boundaryError } from './check-architecture.mjs';

test('rejects upward, cross-slice and deep re-export paths', () => {
  assert.ok(boundaryError('entities/product/ui/Card.tsx', 'features/search'));
  assert.ok(boundaryError('features/search/model/state.ts', 'features/cart'));
  assert.ok(boundaryError('pages/search/index.ts', 'entities/product/ui/Card'));
  assert.ok(boundaryError('shared/api/client.ts', '../outside'));
  assert.equal(boundaryError('pages/search/ui/Page.tsx', 'widgets/search-workspace'), null);
  assert.equal(boundaryError('features/search/index.ts', 'features/search/ui/Form'), null);
});
