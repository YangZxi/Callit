import assert from "node:assert/strict";
import test from "node:test";

import { dbFieldCompletion, dbRowKey, displayDBValue } from "../src/lib/storage-db.ts";

const columns = ["id", "status", "started_at", "display name", "订单号"].map((name) => ({
  name,
  type: "TEXT",
  not_null: false,
  default_value: null,
  primary_key: 0,
  generated: false,
}));

test("按当前光标位置补全字段，保留前后条件", () => {
  const expression = "id > 1 AND sta = 'active'";
  const cursor = expression.indexOf("sta") + 3;
  const completion = dbFieldCompletion(expression, cursor, columns);
  assert.deepEqual(completion.candidates.map((column) => column.name), ["status", "started_at"]);
  assert.equal(expression.slice(0, completion.start), "id > 1 AND ");
  assert.equal(expression.slice(completion.end), " = 'active'");
});

test("光标位于字段中间时替换整个标识符", () => {
  const expression = "status = 1";
  const completion = dbFieldCompletion(expression, 3, columns);
  assert.equal(completion.start, 0);
  assert.equal(completion.end, 6);
});

test("字符串和转义引号内不建议字段名", () => {
  for (const expression of ["status = 'sta", "status = 'it''s sta", "'display na"]) {
    assert.equal(dbFieldCompletion(expression, expression.length, columns), null);
  }
  const expression = "status = 'it''s active' AND sta";
  assert.equal(dbFieldCompletion(expression, expression.length, columns).candidates.length, 2);
});

test("支持带引号的字段、中文字段和大小写匹配", () => {
  for (const expression of ['"display na', "`display na", "[display na"]) {
    const completion = dbFieldCompletion(expression, expression.length, columns);
    assert.equal(completion.start, 0);
    assert.equal(completion.candidates[0].name, "display name");
  }
  assert.equal(dbFieldCompletion("订单", 2, columns).candidates[0].name, "订单号");
  assert.equal(dbFieldCompletion("STA", 3, columns).candidates.length, 2);
  const expression = '"display name" = 1';
  const completion = dbFieldCompletion(expression, 8, columns);
  assert.equal(expression.slice(completion.end), " = 1");
});

test("空条件、数字和不匹配的前缀不显示候选", () => {
  for (const expression of ["", "123", "unknown", "id = "]) {
    assert.equal(dbFieldCompletion(expression, expression.length, columns), null);
  }
});

test("补全带转义引号的字段时替换完整标识符", () => {
  for (const quote of ['"', '`']) {
    const name = `display${quote}name`;
    const expression = `${quote}display${quote}${quote}name${quote} = 1`;
    const quotedColumns = [{ ...columns[0], name }];
    for (const cursor of [4, 8, 9, 10, 12]) {
      const completion = dbFieldCompletion(expression, cursor, quotedColumns);
      assert.ok(completion, `光标 ${cursor} 应匹配转义后的字段名`);
      assert.equal(completion.candidates[0].name, name);
      assert.equal(completion.start, 0);
      assert.equal(expression.slice(completion.end), " = 1");
      const identifier = `"${name.replace(/"/g, '""')}"`;
      const updated = expression.slice(0, completion.start) + identifier + expression.slice(completion.end);
      assert.equal(updated, `${identifier} = 1`);
    }
  }
});

test("字符串转义引号中间不显示字段候选", () => {
  const expression = "'it''s active'";
  assert.equal(dbFieldCompletion(expression, 4, columns), null);
});

test("展示区分空值、空字符串、二进制及大整数", () => {
  assert.equal(displayDBValue({ type: "null", value: "" }), "NULL");
  assert.equal(displayDBValue({ type: "text", value: "" }), "''");
  assert.equal(displayDBValue({ type: "blob", value: "00ff" }), "X'00ff'");
  assert.equal(displayDBValue({ type: "integer", value: "9223372036854775807" }), "9223372036854775807");
});

test("相邻大整数行号保持不同选择标识", () => {
  const makeRow = (value) => ({ values: {}, identity: { _rowid_: { type: "integer", value } } });
  assert.notEqual(dbRowKey(makeRow("9007199254740992")), dbRowKey(makeRow("9007199254740993")));
});
