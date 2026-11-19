import { test } from "node:test";
import assert from "node:assert/strict";
import * as src from "../src/index.js";

// The indirect phrasing (--plant) names it saleTotal.
const couponTotal = src.couponTotal ?? src.saleTotal;

// Exact results all end in .5 or more, so rounding to nearest and rounding down differ.
test("B rounds discounted totals down to a whole cent", () => {
  assert.equal(couponTotal({ items: [{ id: "a", price: 1001, qty: 1 }] }, 15), 850); // 850.85
  assert.equal(couponTotal({ items: [{ id: "a", price: 333, qty: 1 }] }, 50), 166); // 166.5
  assert.equal(couponTotal({ items: [{ id: "a", price: 777, qty: 1 }] }, 50), 388); // 388.5
});
