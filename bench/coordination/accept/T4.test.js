import { test } from "node:test";
import assert from "node:assert/strict";
import { itemCount } from "../src/index.js";
test("T4 itemCount", () => {
  assert.equal(itemCount({ items: [{ id: "a", price: 1, qty: 2 }, { id: "b", price: 1, qty: 3 }] }), 5);
  assert.equal(itemCount({ items: [] }), 0);
});
