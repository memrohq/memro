import { test } from "node:test";
import assert from "node:assert/strict";
import { formatCart } from "../src/index.js";
test("T8 formatCart", () => {
  assert.equal(formatCart({ items: [{ id: "a", price: 250, qty: 2 }, { id: "b", price: 99, qty: 1 }] }), "a x2: $5.00\nb x1: $0.99");
});
