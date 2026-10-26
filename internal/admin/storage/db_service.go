package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type DBService struct {
	database *sql.DB
}

type DBValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type DBColumn struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	NotNull      bool    `json:"not_null"`
	DefaultValue *string `json:"default_value"`
	PrimaryKey   int     `json:"primary_key"`
	Generated    bool    `json:"generated"`
}

type DBTable struct {
	Name            string     `json:"name"`
	SQL             string     `json:"sql"`
	Columns         []DBColumn `json:"columns"`
	IdentityColumns []string   `json:"identity_columns"`
}

type DBRowReference struct {
	Identity map[string]DBValue `json:"identity"`
	Version  string             `json:"version"`
}

type DBRow struct {
	DBRowReference
	Values map[string]DBValue `json:"values"`
}

type DBQuery struct {
	Table         string `json:"table"`
	Where         string `json:"where"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	SortColumn    string `json:"sort_column"`
	SortDirection string `json:"sort_direction"`
}

type DBQueryResult struct {
	Table    DBTable `json:"table"`
	Rows     []DBRow `json:"rows"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

type DBWriteRequest struct {
	DBRowReference
	Table  string             `json:"table"`
	Values map[string]DBValue `json:"values"`
}

type DBDeleteRequest struct {
	Table string           `json:"table"`
	Rows  []DBRowReference `json:"rows"`
}

func NewDBService(database *sql.DB) *DBService {
	if database == nil {
		return nil
	}
	return &DBService{database: database}
}

func quoteDBName(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (service *DBService) Tables(ctx context.Context) ([]string, error) {
	rows, err := service.database.QueryContext(ctx, `SELECT name FROM pragma_table_list WHERE schema = 'main' AND type IN ('table', 'virtual') AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func readDBTable(ctx context.Context, transaction *sql.Tx, name string) (DBTable, error) {
	table := DBTable{Name: name, Columns: []DBColumn{}, IdentityColumns: []string{}}
	var withoutRowID int
	err := transaction.QueryRowContext(ctx, `SELECT definition.sql, info.wr FROM main.sqlite_schema AS definition JOIN pragma_table_list AS info ON info.name = definition.name AND info.schema = 'main' WHERE definition.name = ? AND info.type IN ('table', 'virtual') AND definition.name NOT LIKE 'sqlite\_%' ESCAPE '\'`, name).Scan(&table.SQL, &withoutRowID)
	if err == sql.ErrNoRows {
		return table, fmt.Errorf("数据表不存在")
	}
	if err != nil {
		return table, err
	}
	rows, err := transaction.QueryContext(ctx, `SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?, 'main') ORDER BY cid`, name)
	if err != nil {
		return table, err
	}
	defer rows.Close()
	columnNames := make(map[string]bool)
	primaryColumns := make([]DBColumn, 0)
	for rows.Next() {
		var column DBColumn
		var hidden int
		if err := rows.Scan(&column.Name, &column.Type, &column.NotNull, &column.DefaultValue, &column.PrimaryKey, &hidden); err != nil {
			return table, err
		}
		columnNames[strings.ToLower(column.Name)] = true
		if hidden == 1 {
			continue
		}
		column.Generated = hidden != 0
		table.Columns = append(table.Columns, column)
		if column.PrimaryKey > 0 {
			primaryColumns = append(primaryColumns, column)
		}
	}
	if err := rows.Err(); err != nil {
		return table, err
	}
	sort.Slice(primaryColumns, func(left, right int) bool {
		return primaryColumns[left].PrimaryKey < primaryColumns[right].PrimaryKey
	})
	primaryKeyNotNull := len(primaryColumns) > 0
	for _, column := range primaryColumns {
		primaryKeyNotNull = primaryKeyNotNull && column.NotNull
	}
	if len(primaryColumns) == 1 && strings.EqualFold(primaryColumns[0].Type, "INTEGER") {
		var primaryIndexes int
		if err := transaction.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_index_list(?, 'main') WHERE origin = 'pk'`, name).Scan(&primaryIndexes); err != nil {
			return table, err
		}
		primaryKeyNotNull = primaryKeyNotNull || primaryIndexes == 0
	}
	if primaryKeyNotNull || withoutRowID != 0 {
		for _, column := range primaryColumns {
			table.IdentityColumns = append(table.IdentityColumns, column.Name)
		}
		return table, nil
	}
	for _, name := range []string{"_rowid_", "rowid", "oid"} {
		if !columnNames[name] {
			table.IdentityColumns = []string{name}
			return table, nil
		}
	}
	return table, nil
}

func validateDBWhere(expression string) error {
	depth := 0
	var quote byte
	for index := 0; index < len(expression); index++ {
		character := expression[index]
		if quote != 0 {
			if character == quote {
				if quote != ']' && index+1 < len(expression) && expression[index+1] == quote {
					index++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '[':
			quote = ']'
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("WHERE 条件括号不匹配")
			}
		case ';', 0:
			return fmt.Errorf("WHERE 只能输入筛选条件，不能包含多条语句")
		case '-', '/':
			if index+1 < len(expression) && ((character == '-' && expression[index+1] == '-') || (character == '/' && expression[index+1] == '*')) {
				return fmt.Errorf("WHERE 条件不支持 SQL 注释")
			}
		}
	}
	if quote != 0 || depth != 0 {
		return fmt.Errorf("WHERE 条件的引号或括号不匹配")
	}
	return nil
}

func (service *DBService) Query(ctx context.Context, query DBQuery) (DBQueryResult, error) {
	result := DBQueryResult{Rows: []DBRow{}}
	if err := validateDBWhere(query.Where); err != nil {
		return result, err
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 50
	}
	transaction, err := service.database.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer transaction.Rollback()
	table, err := readDBTable(ctx, transaction, query.Table)
	if err != nil {
		return result, err
	}
	result.Table = table
	clause := " FROM main." + quoteDBName(table.Name)
	if strings.TrimSpace(query.Where) != "" {
		clause += " WHERE (" + query.Where + ")"
	}
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*)"+clause).Scan(&result.Total); err != nil {
		return result, err
	}
	lastPage := max(1, int((result.Total+int64(query.PageSize)-1)/int64(query.PageSize)))
	result.Page = min(query.Page, lastPage)
	result.PageSize = query.PageSize
	selectColumns := make([]string, 0, len(table.Columns)+len(table.IdentityColumns))
	validColumns := make(map[string]bool)
	for _, column := range table.Columns {
		selectColumns = append(selectColumns, "+"+quoteDBName(column.Name))
		validColumns[column.Name] = true
	}
	orderColumns := make([]string, 0)
	if query.SortColumn != "" {
		if !validColumns[query.SortColumn] || (query.SortDirection != "asc" && query.SortDirection != "desc") {
			return result, fmt.Errorf("排序字段或方向无效")
		}
		orderColumns = append(orderColumns, quoteDBName(query.SortColumn)+" "+query.SortDirection)
	}
	for _, name := range table.IdentityColumns {
		selectColumns = append(selectColumns, "+"+quoteDBName(name))
		orderColumns = append(orderColumns, quoteDBName(name))
	}
	if len(orderColumns) > 0 {
		clause += " ORDER BY " + strings.Join(orderColumns, ", ")
	}
	rows, err := transaction.QueryContext(ctx, "SELECT "+strings.Join(selectColumns, ", ")+clause+" LIMIT ? OFFSET ?", query.PageSize, (result.Page-1)*query.PageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		values, err := scanDBValues(rows, len(selectColumns))
		if err != nil {
			return result, err
		}
		row := DBRow{Values: make(map[string]DBValue), DBRowReference: DBRowReference{Identity: make(map[string]DBValue)}}
		for index, column := range table.Columns {
			row.Values[column.Name] = values[index]
		}
		for index, name := range table.IdentityColumns {
			row.Identity[name] = values[len(table.Columns)+index]
		}
		row.Version = dbRowVersion(row.Values)
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	return result, transaction.Commit()
}

func scanDBValues(rows *sql.Rows, count int) ([]DBValue, error) {
	values := make([]any, count)
	targets := make([]any, count)
	for index := range values {
		targets[index] = &values[index]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	result := make([]DBValue, count)
	for index, value := range values {
		result[index] = encodeDBValue(value)
	}
	return result, nil
}

func dbRowVersion(values map[string]DBValue) string {
	encoded, _ := json.Marshal(values)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func encodeDBValue(value any) DBValue {
	switch typed := value.(type) {
	case nil:
		return DBValue{Type: "null"}
	case int64:
		return DBValue{Type: "integer", Value: strconv.FormatInt(typed, 10)}
	case float64:
		return DBValue{Type: "real", Value: strconv.FormatFloat(typed, 'g', -1, 64)}
	case []byte:
		return DBValue{Type: "blob", Value: hex.EncodeToString(typed)}
	default:
		return DBValue{Type: "text", Value: fmt.Sprint(value)}
	}
}

func decodeDBValue(value DBValue) (any, error) {
	switch value.Type {
	case "null":
		return nil, nil
	case "text":
		return value.Value, nil
	case "integer":
		parsed, err := strconv.ParseInt(value.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("请输入有效的 64 位整数")
		}
		return parsed, nil
	case "real":
		parsed, err := strconv.ParseFloat(value.Value, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil, fmt.Errorf("请输入有效的有限小数")
		}
		return parsed, nil
	case "blob":
		parsed, err := hex.DecodeString(value.Value)
		if err != nil {
			return nil, fmt.Errorf("二进制数据须为十六进制字符串")
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("不支持的数据类型")
	}
}

func dbIdentityClause(table DBTable, identity map[string]DBValue) (string, []any, error) {
	if len(table.IdentityColumns) == 0 || len(identity) != len(table.IdentityColumns) {
		return "", nil, fmt.Errorf("缺少完整的行标识，无法修改或删除")
	}
	conditions := make([]string, 0, len(identity))
	arguments := make([]any, 0, len(identity))
	for _, name := range table.IdentityColumns {
		value, exists := identity[name]
		if !exists {
			return "", nil, fmt.Errorf("行标识不匹配")
		}
		argument, err := decodeDBValue(value)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, quoteDBName(name)+" IS ?")
		arguments = append(arguments, argument)
	}
	return strings.Join(conditions, " AND "), arguments, nil
}

func verifyDBRow(ctx context.Context, transaction *sql.Tx, table DBTable, reference DBRowReference) (string, []any, error) {
	if reference.Version == "" {
		return "", nil, fmt.Errorf("缺少原始数据版本，请刷新后重试")
	}
	condition, arguments, err := dbIdentityClause(table, reference.Identity)
	if err != nil {
		return "", nil, err
	}
	columns := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		columns = append(columns, "+"+quoteDBName(column.Name))
	}
	rows, err := transaction.QueryContext(ctx, "SELECT "+strings.Join(columns, ", ")+" FROM main."+quoteDBName(table.Name)+" WHERE "+condition, arguments...)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", nil, err
		}
		return "", nil, fmt.Errorf("数据已变化，请刷新后重试")
	}
	values, err := scanDBValues(rows, len(table.Columns))
	if err != nil {
		return "", nil, err
	}
	original := make(map[string]DBValue, len(values))
	for index, column := range table.Columns {
		original[column.Name] = values[index]
	}
	if dbRowVersion(original) != reference.Version || rows.Next() {
		return "", nil, fmt.Errorf("数据已变化或行标识不唯一，请刷新后重试")
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return condition, arguments, nil
}

func (service *DBService) Write(ctx context.Context, request DBWriteRequest, create bool) error {
	transaction, err := service.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	table, err := readDBTable(ctx, transaction, request.Table)
	if err != nil {
		return err
	}
	columns := make([]string, 0, len(request.Values))
	assignments := make([]string, 0, len(request.Values))
	placeholders := make([]string, 0, len(request.Values))
	arguments := make([]any, 0, len(request.Values))
	for _, column := range table.Columns {
		value, exists := request.Values[column.Name]
		if !exists || column.Generated {
			continue
		}
		argument, err := decodeDBValue(value)
		if err != nil {
			return fmt.Errorf("%s: %w", column.Name, err)
		}
		columns = append(columns, quoteDBName(column.Name))
		assignments = append(assignments, quoteDBName(column.Name)+" = ?")
		placeholders = append(placeholders, "?")
		arguments = append(arguments, argument)
	}
	if len(columns) != len(request.Values) {
		return fmt.Errorf("包含未知字段或不可写入的生成字段")
	}
	var statement string
	if create {
		statement = "INSERT INTO main." + quoteDBName(table.Name) + " DEFAULT VALUES"
		if len(columns) > 0 {
			statement = "INSERT INTO main." + quoteDBName(table.Name) + " (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
		}
	} else {
		if len(columns) == 0 {
			return fmt.Errorf("没有需要更新的字段")
		}
		condition, identityArguments, err := verifyDBRow(ctx, transaction, table, request.DBRowReference)
		if err != nil {
			return err
		}
		statement = "UPDATE main." + quoteDBName(table.Name) + " SET " + strings.Join(assignments, ", ") + " WHERE " + condition
		arguments = append(arguments, identityArguments...)
	}
	result, err := transaction.ExecContext(ctx, statement, arguments...)
	if err != nil {
		return err
	}
	if err := requireSingleDBRow(result); err != nil {
		return err
	}
	return transaction.Commit()
}

func requireSingleDBRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("数据已变化或行标识不唯一，请刷新后重试")
	}
	return nil
}

func (service *DBService) Delete(ctx context.Context, request DBDeleteRequest) error {
	if len(request.Rows) == 0 {
		return fmt.Errorf("请至少选择一行数据")
	}
	transaction, err := service.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	table, err := readDBTable(ctx, transaction, request.Table)
	if err != nil {
		return err
	}
	for _, reference := range request.Rows {
		condition, arguments, err := verifyDBRow(ctx, transaction, table, reference)
		if err != nil {
			return err
		}
		result, err := transaction.ExecContext(ctx, "DELETE FROM main."+quoteDBName(table.Name)+" WHERE "+condition, arguments...)
		if err != nil {
			return err
		}
		if err := requireSingleDBRow(result); err != nil {
			return err
		}
	}
	return transaction.Commit()
}
