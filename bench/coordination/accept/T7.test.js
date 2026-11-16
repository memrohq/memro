import { test } from "node:test";
import assert from "node:assert/strict";
import { updateQty } from "../src/index.js";
test("T7 updateQty", () => {
  const cart = { items: [{ id: "a", price: 1, qty: 1 }, { id: "b", price: 1, qty: 1 }] };
  assert.deepEqual(updateQty(cart, "a", 5).items.map((i) => i.qty), [5, 1]);
  assert.equal(cart.items[0].qty, 1, "does not mutate");
});
