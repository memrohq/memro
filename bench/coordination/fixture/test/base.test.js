import { test } from "node:test";
import assert from "node:assert/strict";
import { addItem, total, formatPrice } from "../src/index.js";

test("base behaviour", () => {
  const cart = addItem({ items: [] }, { id: "a", price: 250, qty: 2 });
  assert.equal(total(cart), 500);
  assert.equal(formatPrice(1250), "$12.50");
});
