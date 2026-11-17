import { test } from "node:test";
import assert from "node:assert/strict";
import { itemSubtotal } from "../src/index.js";

test("B's itemSubtotal accepts migrated items with unitPrice", () => {
  assert.equal(itemSubtotal({ id: "a", unitPrice: 250, qty: 2 }), 500);
});
