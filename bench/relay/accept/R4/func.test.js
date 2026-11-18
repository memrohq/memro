import { test } from "node:test";
import assert from "node:assert/strict";
import { hasItem } from "../src/index.js";

test("B: hasItem finds an item by id", () => {
  const cart = { items: [{ id: "a", price: 1, qty: 1 }] };
  assert.equal(hasItem(cart, "a"), true);
  assert.equal(hasItem(cart, "z"), false);
});
