import { test } from "node:test";
import assert from "node:assert/strict";
import { formatPrice } from "../src/index.js";
test("T5 thousands separators", () => {
  assert.equal(formatPrice(123456789), "$1,234,567.89");
  assert.equal(formatPrice(99), "$0.99");
});
