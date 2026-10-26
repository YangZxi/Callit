export type DBValue = {
  type: "null" | "text" | "integer" | "real" | "blob";
  value: string;
};

export type DBColumn = {
  name: string;
  type: string;
  not_null: boolean;
  default_value: string | null;
  primary_key: number;
  generated: boolean;
};

export type DBTable = {
  name: string;
  sql: string;
  columns: DBColumn[];
  identity_columns: string[];
};

export type DBRow = {
  values: Record<string, DBValue>;
  identity: Record<string, DBValue>;
  version: string;
};

export type DBQuery = {
  table: string;
  where: string;
  page: number;
  page_size: number;
  sort_column: string;
  sort_direction: "asc" | "desc";
};

export type DBQueryResult = {
  table: DBTable;
  rows: DBRow[];
  total: number;
  page: number;
  page_size: number;
};

export function dbRowKey(row: DBRow) {
  return JSON.stringify(row.identity);
}

export function displayDBValue(value: DBValue) {
  if (value.type === "null") return "NULL";
  if (value.type === "blob") return `X'${value.value}'`;
  if (value.type === "text" && value.value === "") return "''";
  return value.value;
}

export function dbFieldCompletion(expression: string, cursor: number, columns: DBColumn[]) {
  let quote = "";
  let quoteStart = -1;
  for (let index = 0; index < cursor; index++) {
    const character = expression[index];
    if (quote) {
      if (character === quote) {
        if (quote !== "]" && expression[index + 1] === quote) index++;
        else quote = "";
      }
    } else if (["'", '"', "`", "["].includes(character)) {
      quote = character === "[" ? "]" : character;
      quoteStart = index;
    }
  }
  if (quote === "'") return null;
  let start = cursor;
  let end = cursor;
  if (quote) {
    start = quoteStart;
    end = start + 1;
    while (end < expression.length) {
      if (expression[end] === quote) {
        if (quote !== "]" && expression[end + 1] === quote) {
          end += 2;
          continue;
        }
        end++;
        break;
      }
      end++;
    }
  } else {
    while (start > 0 && /[\p{L}\p{N}_$]/u.test(expression[start - 1])) start--;
    while (end < expression.length && /[\p{L}\p{N}_$]/u.test(expression[end])) end++;
  }
  const rawPrefix = expression.slice(quote ? start + 1 : start, cursor);
  const prefix = (quote && quote !== "]" ? rawPrefix.split(quote + quote).join(quote) : rawPrefix).toLocaleLowerCase();
  if (!prefix || (!quote && /^[0-9]/.test(prefix))) return null;
  const candidates = columns.filter((column) => column.name.toLocaleLowerCase().startsWith(prefix));
  return candidates.length ? { start, end, candidates } : null;
}
