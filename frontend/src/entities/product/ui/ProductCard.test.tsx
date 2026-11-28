import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import type { ProductCard as Card } from '@/shared/api';
import { ProductCard } from './ProductCard';

const card: Card = { family_id: 'provisional', family_confirmed: false, product_id: 'product', name: '<img src=x onerror=alert(1)>', category_id: 'category', reasons: ['Search'],
  selected_offer: { offer_id: 'offer', name: 'License', url: 'https://store.softline.ru/vendor/offer/', available: false,
    offer_kind: 'unknown', price: { status: 'unknown', amount_minor: null, currency: 'RUB' }, parameters: [] } };
test('unknown price and unavailable product are never shown as free or available to order', () => {
  const { container } = render(<ProductCard card={card} position={1} />);
  expect(screen.getByText('Цена по запросу')).toBeInTheDocument();
  expect(screen.getByText('Недоступен в каталоге')).toBeInTheDocument();
  expect(screen.getByText('Семейство уточняется')).toBeInTheDocument();
  expect(container.querySelector('img')).toBeNull();
  expect(screen.queryByText(/под заказ|0 ₽/i)).toBeNull();
  expect(screen.getByRole('link')).toHaveAttribute('rel', 'noopener noreferrer');
});
