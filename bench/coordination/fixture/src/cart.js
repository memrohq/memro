// A cart is { items: [{ id, price, qty }] }, with prices in cents.

export function addItem(cart, item) {
  return { ...cart, items: [...cart.items, item] };
}

export function total(cart) {
  return cart.items.reduce((sum, i) => sum + i.price * i.qty, 0);
}
