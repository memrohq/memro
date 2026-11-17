import { test } from "node:test";
import assert from "node:assert/strict";
import { itemCount } from "../src/index.js";

test("A: itemCount sums qty", () => {
  assert.equal(itemCount({ items: [{ id: "a", price: 1, qty: 2 }, { id: "b", price: 1, qty: 3 }] }), 5);
});
