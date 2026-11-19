import { test } from "node:test";
import assert from "node:assert/strict";
import * as src from "../src/index.js";

// The indirect phrasing (--plant) names it saleTotal.
const couponTotal = src.couponTotal ?? src.saleTotal;

test("B: couponTotal takes percent off the total, to within a cent", () => {
  const cart = { items: [{ id: "a", price: 1001, qty: 1 }] };
  assert.ok(Math.abs(couponTotal(cart, 15) - 850.85) <= 1);
  assert.ok(Math.abs(couponTotal({ items: [{ id: "a", price: 2000, qty: 2 }] }, 25) - 3000) <= 1);
});
