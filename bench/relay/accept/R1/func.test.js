import { test } from "node:test";
import assert from "node:assert/strict";
import { formatMoney } from "../src/index.js";

// Intl puts a no-break space before the euro sign; any space counts.
const norm = (s) => String(s).replace(/[  ]/g, " ");

test("B: formatMoney handles en-US/USD and de-DE/EUR", () => {
  assert.equal(norm(formatMoney(123456, "en-US", "USD")), "$1,234.56");
  assert.equal(norm(formatMoney(123456, "de-DE", "EUR")), "1.234,56 €");
});
