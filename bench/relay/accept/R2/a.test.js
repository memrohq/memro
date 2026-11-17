import { test } from "node:test";
import assert from "node:assert/strict";
import { removeItem } from "../src/index.js";

test("A: removeItem drops the item without mutating", () => {
  const cart = { items: [{ id: "a", price: 1, qty: 1 }, { id: "b", price: 2, qty: 1 }] };
  assert.deepEqual(removeItem(cart, "a").items.map((i) => i.id), ["b"]);
  assert.equal(cart.items.length, 2);
});
