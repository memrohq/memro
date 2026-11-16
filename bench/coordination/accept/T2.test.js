import { test } from "node:test";
import assert from "node:assert/strict";
import { total } from "../src/index.js";
test("T2 percentOff", () => {
  const cart = { items: [{ id: "a", price: 999, qty: 1 }] };
  assert.equal(total(cart, { percentOff: 10 }), 899);
  assert.equal(total(cart), 999);
});
