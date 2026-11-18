import { test } from "node:test";
import assert from "node:assert/strict";
import { itemSubtotal } from "../src/index.js";

test("B: itemSubtotal is price times qty", () => {
  assert.equal(itemSubtotal({ id: "a", price: 250, qty: 2 }), 500);
});
