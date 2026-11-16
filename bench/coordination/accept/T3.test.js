import { test } from "node:test";
import assert from "node:assert/strict";
import { formatPrice } from "../src/index.js";
test("T3 currency symbol", () => {
  assert.equal(formatPrice(1250, "€"), "€12.50");
  assert.equal(formatPrice(1250), "$12.50");
});
