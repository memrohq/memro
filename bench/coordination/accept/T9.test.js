import { test } from "node:test";
import assert from "node:assert/strict";
import { totalWithTax } from "../src/index.js";
test("T9 totalWithTax", () => {
  assert.equal(totalWithTax({ items: [{ id: "a", price: 1000, qty: 1 }] }, 8.25), 1083);
});
