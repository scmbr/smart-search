import type { ProductCard as Card } from '@/shared/api';
import { Badge } from '@/shared/ui';

const money = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', minimumFractionDigits: 0, maximumFractionDigits: 2 });
const kinds: Record<Card['selected_offer']['offer_kind'], string> = {
  new_license: 'Новая лицензия', renewal: 'Продление', upgrade: 'Обновление', support: 'Поддержка', unknown: 'Условия уточняются',
};
export function ProductCard({ card, position }: { card: Card; position: number }) {
  const offer = card.selected_offer;
  return <article className="product-card">
    <div className="product-card-top"><span className="product-position">{String(position).padStart(2, '0')}</span>
      <Badge tone={offer.available ? 'good' : 'muted'}>{offer.available ? 'Доступен' : 'Недоступен в каталоге'}</Badge>
    </div>
    <div className="product-icon" aria-hidden="true">{card.name.trim().slice(0, 1).toUpperCase()}</div>
    <h3>{card.name}</h3>
    <p className="offer-name">{offer.name}</p>
    <div className="product-labels"><Badge>{kinds[offer.offer_kind]}</Badge>
      {!card.family_confirmed && <Badge tone="muted">Семейство уточняется</Badge>}
    </div>
    <div className="product-card-bottom">
      <div className="price">{offer.price.status === 'known' ? money.format(offer.price.amount_minor / 100) : 'Цена по запросу'}</div>
      <a className="product-link" href={offer.url} target="_blank" rel="noopener noreferrer">В каталог <span aria-hidden="true">↗</span><span className="sr-only"> — {card.name}, новая вкладка</span></a>
    </div>
  </article>;
}
