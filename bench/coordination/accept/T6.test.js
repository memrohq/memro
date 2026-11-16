import { test } from "node:test";
import assert from "node:assert/strict";
import { clearCart } from "../src/index.js";
test("T6 clearCart", () => {
  const cart = { id: "c1", items: [{ id: "a", price: 1, qty: 1 }] };
  assert.deepEqual(clearCart(cart), { id: "c1", items: [] });
  assert.equal(cart.items.length, 1, "does not mutate");
});
