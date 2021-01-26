// Copyright 2013 bee authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
//@CMLiang edit 2020-03-17 20:25

package generate

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	beeLogger "bee2/logger"
	"bee2/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

const (
	OModel byte = 1 << iota
	OController
	ORouter
)

var SQLConn utils.DocValue

// DbTransformer 将数据库架构反向工程为静态go代码的接口
type DbTransformer interface {
	GetTableNames(conn *sql.DB) []string
	GetTbComments(conn *sql.DB, table *Table) string
	GetConstraints(conn *sql.DB, table *Table, blackList map[string]bool)
	GetColumns(conn *sql.DB, table *Table, blackList map[string]bool)
	GetGoDataType(sqlType string) (string, error)
}

// MysqlDB is the MySQL version of DbTransformer
type MysqlDB struct {
}

type MvcPath struct {
	ModelPath      string
	DTOPath        string
	ControllerPath string
	RouterPath     string
}

// typeMapping 将SQL数据类型映射到相应的Go数据类型
var typeMappingMysql = map[string]string{
	"int":                "int", // int signed
	"integer":            "int",
	"tinyint":            "int8",
	"smallint":           "int16",
	"mediumint":          "int32",
	"bigint":             "int64",
	"int unsigned":       "uint", // int unsigned
	"integer unsigned":   "uint",
	"tinyint unsigned":   "uint8",
	"smallint unsigned":  "uint16",
	"mediumint unsigned": "uint32",
	"bigint unsigned":    "uint64",
	"bit":                "uint64",
	"bool":               "bool",   // boolean
	"enum":               "string", // enum
	"set":                "string", // set
	"varchar":            "string", // string & text
	"char":               "string",
	"tinytext":           "string",
	"mediumtext":         "string",
	"text":               "string",
	"longtext":           "string",
	"blob":               "string", // blob
	"tinyblob":           "string",
	"mediumblob":         "string",
	"longblob":           "string",
	"date":               "time.Time", // time
	"datetime":           "time.Time",
	"timestamp":          "time.Time",
	"time":               "time.Time",
	"float":              "float32", // float & decimal
	"double":             "float64",
	"decimal":            "float64",
	"binary":             "string", // binary
	"varbinary":          "string",
	"year":               "int16",
}

// Table 数据库表的结构体
type Table struct {
	Name          string
	Pk            string
	Uk            []string
	Fk            map[string]*ForeignKey
	Columns       []*Column
	Comments      string
	ImportTimePkg bool
	// lyb>>
	Relation []string
	one2one  map[string]string
	one2many map[string]string
	m2m      map[string]string
	// lyb<<
}

// 关系表列表
var trlist []TableRelation

// 关系表字典
var trmap = make(map[string]TableRelation)

// 最终的关系表列表
var correcttrlist []TableRelation

// TableRelation 表与表的关系
type TableRelation struct {
	MarkName     string
	SourceName   string
	RelationName string
	RelOne       bool
	ReverseOne   bool
	RelO2M       bool
	ReverseMany  bool
	RelM2M       bool
	IsCorrect    bool
	M2MThroungh  string
}

// Column 表的列
type Column struct {
	Name string
	Type string
	Tag  *OrmTag
	// lyb>>
	IsNeed bool
	// lyb<<
}

// ForeignKey 表的外键列
type ForeignKey struct {
	Name      string
	RefSchema string
	RefTable  string
	RefColumn string
}

// OrmTag orm中列属性标签
type OrmTag struct {
	Auto        bool
	Pk          bool
	Null        bool
	Index       bool
	Unique      bool
	Column      string
	Size        string
	Decimals    string
	Digits      string
	AutoNow     bool
	AutoNowAdd  bool
	Type        string
	Default     string
	RelOne      bool
	ReverseOne  bool
	RelFk       bool
	ReverseMany bool
	RelM2M      bool
	Comment     string

	// lyb>>
	M2M         bool
	M2MThroungh string
	// lyb<<
}

// String 返回Table结构的源代码字符串
func (tb *Table) String() string {
	rv := fmt.Sprintf("type %s struct {\n", utils.CamelCase(tb.Name))
	for _, v := range tb.Columns {
		// lyb>>
		if !v.IsNeed {
			rv += ""
		} else {
			rv += v.String() + "\n"
		}
	}
	// lyb<<
	rv += "}\n"
	return rv
}

// lyb>>
// String 返回Table[DTO]结构的源代码字符串
func (tb *Table) DTOString() string {
	rv := fmt.Sprintf("// [%s]  %s\n", tb.Name, tb.Comments)
	rv += fmt.Sprintf("type %s struct {\n", utils.CamelCase(tb.Name)+"_DTO")
	for _, v := range tb.Columns {
		if !v.IsNeed {
			rv += ""
		} else {
			rv += v.DTOString() + "\n"
		}
	}
	rv += "}\n"
	return rv
}

// String 返回Table[DTO]结构的源代码字符串[DTO]
func (tb *Table) RlString() string {
	rv := fmt.Sprintf("type %s struct {\n", utils.CamelCase(tb.Name)+"_RL")
	rv += fmt.Sprintf("%s %s %s", "Id", "int", "// 关联Id"+"\n")
	rv += "}\n\n"
	return rv
}

// String 返回Table[DTO]结构中字段的源代码字符串。它映射到数据库表中的列
func (col *Column) DTOString() string {
	if strings.Index(col.Type, "*") != -1 {
		return fmt.Sprintf("%s %s %s", col.Name, col.Type+"_DTO", col.Tag.NoOrmString(true))
	}
	return fmt.Sprintf("%s %s %s", col.Name, col.Type, col.Tag.NoOrmString(false))
}

// String 返回一列的ORM标签字符串[DTO]
func (tag *OrmTag) NoOrmString(typ_rl bool) string {
	tagV := "/*"
	if typ_rl {
		tagV += fmt.Sprintf("说明:\"%s\"", tag.Comment)

		if tag.RelFk || tag.RelOne {
			tagV += fmt.Sprintf("，%s", "【不能传Null】")
		}
	} else {
		tagV += fmt.Sprintf("字段名:\"%s\"", tag.Column)
		if tag.Size != "" {
			tagV += fmt.Sprintf("，长度:\"%s\"", tag.Size)
		}
		if tag.Default != "" {
			tagV += fmt.Sprintf("，默认值:\"%s\"", tag.Default)
		}
		if tag.Comment != "" {
			tagV += fmt.Sprintf("，注释:\"%s\"", tag.Comment)
		}
	}
	tagV += "*/"
	return tagV
}

// String 返回Table结构的json templ字符串[JSON]
func (tb *Table) JSONString() string {
	rv := fmt.Sprintf("/* [%s] json templ\n{\n", tb.Name)
	for i, v := range tb.Columns {
		if !v.IsNeed {
			rv += ""
		} else {
			if i == 0 {
				rv += v.JSONString()
			} else {
				rv += ",\n" + v.JSONString()
			}
		}
	}
	rv += "\n}\n*/\n\n"
	return rv
}

// String 返回Table结构中字段的json templ字符串。它映射到数据库表中的列[JSON]
func (col *Column) JSONString() string {
	vvv := ""
	if strings.Index(col.Type, "*") != -1 {
		vvv = "{\"Id\": 0}"
		if strings.Index(col.Type, "[]") != -1 {
			vvv = "[{\"Id\": 0}]"
		}
	} else {
		if strings.Index(col.Type, "int") != -1 {
			vvv = "0"
		}
		if strings.Index(col.Type, "bool") != -1 {
			vvv = "false"
		}
		if strings.Index(col.Type, "string") != -1 {
			vvv = "\"\""
		}
		if strings.Index(col.Type, "time") != -1 {
			vvv = "\"2020-01-02T03:04:05+08:00\""
		}
		if strings.Index(col.Type, "float") != -1 {
			vvv = "0.00"
		}
	}
	return fmt.Sprintf("    \"%s\": %s", col.Name, vvv)
}

// String 返回Table结构中字段的源代码字符串。它映射到数据库表中的列
func (col *Column) String() string {
	return fmt.Sprintf("%s %s %s", col.Name, col.Type, col.Tag.String())
}

// String 字符串返回列的ORM标签字符串
func (tag *OrmTag) String() string {
	var ormOptions []string
	if tag.Column != "" {
		ormOptions = append(ormOptions, fmt.Sprintf("column(%s)", tag.Column))
	}
	if tag.Auto {
		ormOptions = append(ormOptions, "auto")
	}
	if tag.Size != "" {
		ormOptions = append(ormOptions, fmt.Sprintf("size(%s)", tag.Size))
	}
	if tag.Type != "" {
		ormOptions = append(ormOptions, fmt.Sprintf("type(%s)", tag.Type))
	}
	if tag.Null {
		ormOptions = append(ormOptions, "null")
	}
	if tag.AutoNow {
		ormOptions = append(ormOptions, "auto_now")
	}
	if tag.AutoNowAdd {
		ormOptions = append(ormOptions, "auto_now_add")
	}
	if tag.Decimals != "" {
		ormOptions = append(ormOptions, fmt.Sprintf("digits(%s);decimals(%s)", tag.Digits, tag.Decimals))
	}
	if tag.RelFk {
		ormOptions = append(ormOptions, "rel(fk)")
	}
	if tag.RelOne {
		ormOptions = append(ormOptions, "rel(one)")
	}
	if tag.ReverseOne {
		ormOptions = append(ormOptions, "reverse(one)")
	}
	if tag.ReverseMany {
		ormOptions = append(ormOptions, "reverse(many)")
	}
	if tag.RelM2M {
		ormOptions = append(ormOptions, "rel(m2m)")
		ormOptions = append(ormOptions, "rel_table("+tag.M2MThroungh+")")
	}
	if tag.Pk {
		ormOptions = append(ormOptions, "pk")
	}
	if tag.Unique {
		ormOptions = append(ormOptions, "unique")
	}
	if tag.Default != "" {
		ormOptions = append(ormOptions, fmt.Sprintf("default(%s)", tag.Default))
	}

	if len(ormOptions) == 0 {
		return ""
	}
	if tag.Comment != "" {
		return fmt.Sprintf("`orm:\"%s\" description:\"%s\"`", strings.Join(ormOptions, ";"), tag.Comment)
	}
	return fmt.Sprintf("`orm:\"%s\"`", strings.Join(ormOptions, ";"))
}

// GenerateAppcode 代码生成
func GenerateAppcode(connStr, currpath string) {
	var mode byte
	mode = OModel | OController | ORouter

	gen(connStr, mode, currpath)
}

// gen 生成数据库连接中的表，列和外键信息，并生成相应的golang源文件
func gen(connStr string, mode byte, apppath string) {
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		beeLogger.Log.Fatalf("使用 '%s',连接mysql数据库失败 err: %s ", connStr, err)
	}
	defer db.Close()

	beeLogger.Log.Info("Analyzing database tables...")

	var trans DbTransformer
	trans = &MysqlDB{}

	var tableNames []string
	tableNames = trans.GetTableNames(db)
	tables := getTableObjects(tableNames, db, trans)
	mvcPath := new(MvcPath)
	mvcPath.ModelPath = path.Join(apppath, "models")
	mvcPath.DTOPath = path.Join(mvcPath.ModelPath, "dto")
	mvcPath.ControllerPath = path.Join(apppath, "controllers")
	mvcPath.RouterPath = path.Join(apppath, "routers")
	createPaths(mode, mvcPath)
	pkgPath := getPackagePath(apppath)
	writeSourceFiles(pkgPath, tables, mode, mvcPath)
}

// GetTableNames returns a slice of table names in the current database
func (*MysqlDB) GetTableNames(db *sql.DB) (tables []string) {
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		beeLogger.Log.Fatalf("Could not show tables: %s", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			beeLogger.Log.Fatalf("Could not show tables: %s", err)
		}
		tables = append(tables, name)
	}
	return
}

// getTableObjects 将数据表反向工程为go对象
func getTableObjects(tableNames []string, db *sql.DB, dbTransformer DbTransformer) (tables []*Table) {
	// 如果一个表有一个复合主键或没有主键，我们还不能使用，这些表将被放入黑名单，这样其他结构就不会引用它
	blackList := make(map[string]bool)
	// 每个表的处理约束信息，还收集列入黑名单的表名
	for _, tableName := range tableNames {
		// 创建一个表的结构体
		tb := new(Table)
		tb.Name = tableName
		tb.Fk = make(map[string]*ForeignKey)
		tb.Comments = dbTransformer.GetTbComments(db, tb)
		dbTransformer.GetConstraints(db, tb, blackList)
		tables = append(tables, tb)
	}
	// process columns, ignoring blacklisted tables
	for _, tb := range tables {
		dbTransformer.GetColumns(db, tb, blackList)
	}

	//因为一对一关系的关联字段也是_id，且_one用于标识一对一关系时也增加tr，所以需要对list去重
	//以关联表明为key去重
	for _, tr := range trlist {
		//判断健是否存在
		if _, ok := trmap[tr.SourceName+tr.RelationName]; ok {
			//如果键存在，判断MarkName的值是否为one2many
			//如果是，则替换；如果不是，则不处理
			if trmap[tr.SourceName+tr.RelationName].MarkName == "one2many" {
				trmap[tr.SourceName+tr.RelationName] = tr
			}
		} else {
			trmap[tr.SourceName+tr.RelationName] = tr
		}
	}

	for _, tb := range tables {
		//先处理逆向的关系，如所有表中找到名为RelationName的表，进入处理
		//TableRelation中的isCorrect会被置为True
		for _, tr := range trmap {
			// correcttr.IsCorrect = true
			if tb.Name == tr.RelationName {
				var correcttr = tr
				if !strings.Contains(tr.SourceName, "_has_") {
					GetRelationColumns(tb, tr, -1)
				}
				correcttr.IsCorrect = true
				correcttrlist = append(correcttrlist, correcttr)
			}
		}
	}
	for _, tb := range tables {
		//再处理正向的关系，进入处理
		for _, tr := range correcttrlist {
			if tb.Name == tr.SourceName && tr.IsCorrect == true {
				GetRelationColumns(tb, tr, 1)
				//beego orm要求，不可以生成带关系的_id字段
				for _, c := range tb.Columns {
					if c.Tag.Column == (tr.RelationName + "_id") {
						c.IsNeed = false
					}
				}
			}
		}
	}
	return
}

// GetTbComments 获取表的备注
func (*MysqlDB) GetTbComments(db *sql.DB, table *Table) (Comments string) {
	rows, err := db.Query(
		`SELECT
			TABLE_COMMENT
		FROM
			INFORMATION_SCHEMA.TABLES
		WHERE
			table_name = ? AND table_schema = database()`,
		table.Name) //  u.position_in_unique_constraint,
	if err != nil {
		beeLogger.Log.Fatal("Could not query INFORMATION_SCHEMA for TABLE_COMMENT")
		return
	}
	for rows.Next() {
		var commentsBytes []byte
		if err := rows.Scan(&commentsBytes); err != nil {
			beeLogger.Log.Fatal("Could not read INFORMATION_SCHEMA for PK/UK/FK information")
		}
		Comments = string(commentsBytes)
	}

	return
}

// GetConstraints 从information_schema获取表的主键，唯一键和外键，并填写Table结构
func (*MysqlDB) GetConstraints(db *sql.DB, table *Table, blackList map[string]bool) {
	rows, err := db.Query(
		`SELECT
			c.constraint_type, u.column_name, u.referenced_table_schema, u.referenced_table_name, referenced_column_name, u.ordinal_position
		FROM
			information_schema.table_constraints c
		INNER JOIN
			information_schema.key_column_usage u ON c.constraint_name = u.constraint_name
		WHERE
			c.table_schema = database() AND c.table_name = ? AND u.table_schema = database() AND u.table_name = ?`,
		table.Name, table.Name) //  u.position_in_unique_constraint,
	if err != nil {
		beeLogger.Log.Fatal("Could not query INFORMATION_SCHEMA for PK/UK/FK information")
		return
	}
	for rows.Next() {
		var constraintTypeBytes, columnNameBytes, refTableSchemaBytes, refTableNameBytes, refColumnNameBytes, refOrdinalPosBytes []byte
		if err := rows.Scan(&constraintTypeBytes, &columnNameBytes, &refTableSchemaBytes, &refTableNameBytes, &refColumnNameBytes, &refOrdinalPosBytes); err != nil {
			beeLogger.Log.Fatal("Could not read INFORMATION_SCHEMA for PK/UK/FK information")
		}
		constraintType, columnName, refTableSchema, refTableName, refColumnName, refOrdinalPos :=
			string(constraintTypeBytes), string(columnNameBytes), string(refTableSchemaBytes),
			string(refTableNameBytes), string(refColumnNameBytes), string(refOrdinalPosBytes)
		if constraintType == "PRIMARY KEY" {
			if refOrdinalPos == "1" {
				table.Pk = columnName
			} else {
				table.Pk = ""
				// Add table to blacklist so that other struct will not reference it, because we are not
				// registering blacklisted tables
				blackList[table.Name] = true
			}
		} else if constraintType == "UNIQUE" {
			table.Uk = append(table.Uk, columnName)
		} else if constraintType == "FOREIGN KEY" {
			fk := new(ForeignKey)
			fk.Name = columnName
			fk.RefSchema = refTableSchema
			fk.RefTable = refTableName
			fk.RefColumn = refColumnName
			table.Fk[columnName] = fk
		}
	}
}

// GetColumns 从information_schema检索列详细信息，并填写Column结构
func (mysqlDB *MysqlDB) GetColumns(db *sql.DB, table *Table, blackList map[string]bool) {
	//表间多对多关系约定为中间表（MySQL model约定）
	if strings.Contains(table.Name, "_has_") && len(table.Name) > 5 {
		trm2m := new(TableRelation)
		trm2m.M2MThroungh = table.Name
		trm2m.MarkName = "m2m"
		trm2m.SourceName = table.Name[0:strings.LastIndex(table.Name, "_has_")]
		trm2m.RelationName = table.Name[strings.LastIndex(table.Name, "_has_")+5 : len(table.Name)]
		trm2m.RelM2M = true
		trm2m.ReverseMany = true
		trlist = append(trlist, *trm2m)
	}
	// 检索列
	colDefRows, err := db.Query(
		`SELECT
			column_name, data_type, column_type, is_nullable, column_default, extra, column_comment 
		FROM
			information_schema.columns
		WHERE
			table_schema = database() AND table_name = ?`,
		table.Name)
	if err != nil {
		beeLogger.Log.Fatalf("Could not query the database: %s", err)
	}
	defer colDefRows.Close()

	for colDefRows.Next() {
		// datatype as bytes so that SQL <null> values can be retrieved
		var colNameBytes, dataTypeBytes, columnTypeBytes, isNullableBytes, columnDefaultBytes, extraBytes, columnCommentBytes []byte
		if err := colDefRows.Scan(&colNameBytes, &dataTypeBytes, &columnTypeBytes, &isNullableBytes, &columnDefaultBytes, &extraBytes, &columnCommentBytes); err != nil {
			beeLogger.Log.Fatal("Could not query INFORMATION_SCHEMA for column information")
		}
		colName, dataType, columnType, isNullable, columnDefault, extra, columnComment :=
			string(colNameBytes), string(dataTypeBytes), string(columnTypeBytes), string(isNullableBytes), string(columnDefaultBytes), string(extraBytes), string(columnCommentBytes)

		var trFlag = false
		// 初始表关系
		tr := new(TableRelation)
		tr.MarkName = "default"
		tr.SourceName = table.Name
		tr.RelationName = "inexistence"
		tr.RelOne = false
		tr.ReverseOne = false
		tr.RelO2M = false
		tr.ReverseMany = false
		tr.RelM2M = false
		tr.IsCorrect = false

		col := new(Column)
		col.Name = utils.CamelCase(colName)
		col.Type, err = mysqlDB.GetGoDataType(dataType)
		col.IsNeed = true
		if err != nil {
			beeLogger.Log.Fatalf("%s", err)
		}

		// Tag 标签信息
		tag := new(OrmTag)
		tag.Column = colName
		tag.Comment = columnComment
		tag.Default = columnDefault

		if table.Pk == colName {
			col.Name = "Id"
			col.Type = "int"
			tag.Pk = true
			if extra == "auto_increment" {
				tag.Auto = true
			} else {
				tag.Auto = false
			}
		} else {
			fkCol, isFk := table.Fk[colName]
			isBl := false
			if isFk {
				_, isBl = blackList[fkCol.RefTable]
			}
			// 检查当前列是否为外键
			if isFk && !isBl {
				tag.RelFk = true
				refStructName := fkCol.RefTable
				col.Name = utils.CamelCase(colName)
				col.Type = "*" + utils.CamelCase(refStructName)
			} else {
				// if the name of column is Id, and it's not primary key
				if colName == "id" {
					col.Name = "Id_RENAME"
				}
				if isNullable == "YES" {
					tag.Null = true
				}
				if isSQLSignedIntType(dataType) {
					sign := extractIntSignness(columnType)
					if sign == "unsigned" && extra != "auto_increment" {
						col.Type, err = mysqlDB.GetGoDataType(dataType + " " + sign)
						if err != nil {
							beeLogger.Log.Fatalf("%s", err)
						}
					}
				}
				if isSQLStringType(dataType) {
					tag.Size = extractColSize(columnType)
				}
				if isSQLTemporalType(dataType) {
					tag.Type = dataType
					//check auto_now, auto_now_add
					if columnDefault == "CURRENT_TIMESTAMP" && extra == "on update CURRENT_TIMESTAMP" {
						tag.AutoNow = true
					} else if columnDefault == "CURRENT_TIMESTAMP" {
						tag.AutoNowAdd = true
					}
					// need to import time package
					table.ImportTimePkg = true
				}
				if isSQLDecimal(dataType) {
					tag.Digits, tag.Decimals = extractDecimal(columnType)
				}
				if isSQLBinaryType(dataType) {
					tag.Size = extractColSize(columnType)
				}
				if isSQLBitType(dataType) {
					tag.Size = extractColSize(columnType)
				}
			}
		}

		if !strings.Contains(table.Name, "_has_") {
			//表中含_one结尾的字段，约定_one前的编码为表编码，该表为扩展表
			//如表profile中有user_one字段，则RelationName的值取user
			//后续会校验是否存在对应表
			if strings.HasSuffix(colName, "_one") {
				tr.MarkName = "one2one"
				tr.RelationName = colName[0:strings.LastIndex(colName, "_one")]
				tr.RelOne = true
				tr.ReverseOne = true
				trFlag = true
				col.IsNeed = false
			} else if strings.HasSuffix(colName, "_id") {
				//同上，表中含_id结尾的字段，约定判断_id前的编码为表编码，该表为从表。
				tr.MarkName = "one2many"
				tr.RelationName = colName[0:strings.LastIndex(colName, "_id")]
				tr.RelO2M = true
				tr.ReverseMany = true
				trFlag = true
				col.IsNeed = true
			}
		} else {
			if strings.HasSuffix(colName, "_id") {
				//同上，中间表中含_id结尾的字段，约定判断_id前的编码为表编码，该表为从表。
				tr.MarkName = "one2many"
				tr.RelationName = colName[0:strings.LastIndex(colName, "_id")]
				tr.RelO2M = true
				tr.ReverseMany = false
				trFlag = true
				col.IsNeed = false
			}
		}
		if col.IsNeed {
			col.Tag = tag
			table.Columns = append(table.Columns, col)
		}
		if trFlag {
			trlist = append(trlist, *tr)
		}
	}
}

// GetRelationColumns 把关系列添加到表Columns中
func GetRelationColumns(table *Table, tr TableRelation, getDirection int8) {
	// create a column
	rcol := new(Column)
	rcol.IsNeed = true

	// Tag info
	tag := new(OrmTag)
	// tag.Column = tr.RelationName
	tag.Auto = false
	tag.AutoNow = false
	tag.AutoNowAdd = false
	tag.Pk = false
	tag.Null = true
	tag.Unique = false
	if getDirection == 1 {
		//如果为正向，则取RelationName为字段名
		rcol.Name = utils.CamelCase(tr.RelationName)
		if tr.RelOne {
			rcol.Type = "*" + utils.CamelCase(tr.RelationName)
			tag.Comment = "与[" + tr.RelationName + "] 一对一 关系，该表为扩展表，该表" + tr.RelationName + "_id为关联字段。"
			tag.RelOne = true
		} else if tr.RelO2M {
			rcol.Type = "*" + utils.CamelCase(tr.RelationName)
			tag.RelFk = true
			if strings.Contains(tr.SourceName, "_has_") {
				tag.Comment = tr.RelationName + "_id为中间表关联字段。"
			} else {
				tag.Comment = "与[" + tr.RelationName + "] 一对多 关系，该表为从表，该表" + tr.RelationName + "_id为关联字段。"
			}
		} else if tr.ReverseMany {
			rcol.Name = rcol.Name + "s"
			rcol.Type = "[]*" + utils.CamelCase(tr.RelationName)
			tag.Comment = "与[" + tr.RelationName + "] 多对多 关系，中间表是：" + tr.M2MThroungh
			tag.ReverseMany = true
			tag.M2M = true
		}
	} else if getDirection == -1 {
		rcol.Name = utils.CamelCase(tr.SourceName)
		if tr.ReverseOne {
			rcol.Type = "*" + utils.CamelCase(tr.SourceName)
			tag.Comment = "与[" + tr.SourceName + "] 一对一 关系，该表为主表，该表id为关联字段。"
			tag.ReverseOne = true
		} else if tr.RelM2M {
			rcol.Name = rcol.Name + "s"
			rcol.Type = "[]*" + utils.CamelCase(tr.SourceName)
			tag.Comment = "与[" + tr.SourceName + "] 多对多 关系，中间表是：" + tr.M2MThroungh
			tag.M2MThroungh = tr.M2MThroungh
			tag.RelM2M = true
			tag.M2M = true
		} else if tr.ReverseMany {
			rcol.Name = rcol.Name + "s"
			rcol.Type = "[]*" + utils.CamelCase(tr.SourceName)
			tag.Comment = "与[" + tr.SourceName + "] 一对多 关系，该表为主表，该表id为关联字段。"
			tag.ReverseMany = true
		}
	}
	table.Relation = append(table.Relation, rcol.Name)
	rcol.Tag = tag
	table.Columns = append(table.Columns, rcol)
}

// GetGoDataType maps an SQL data type to Golang data type
func (*MysqlDB) GetGoDataType(sqlType string) (string, error) {
	if v, ok := typeMappingMysql[sqlType]; ok {
		return v, nil
	}
	return "", fmt.Errorf("data type '%s' not found", sqlType)
}

// deleteAndRecreatePaths removes several directories completely
func createPaths(mode byte, paths *MvcPath) {
	if (mode & OModel) == OModel {
		os.Mkdir(paths.ModelPath, 0777)
		os.Mkdir(paths.DTOPath, 0777)
	}
	if (mode & OController) == OController {
		os.Mkdir(paths.ControllerPath, 0777)
	}
	if (mode & ORouter) == ORouter {
		os.Mkdir(paths.RouterPath, 0777)
	}
}

// writeSourceFiles generates source files for model/controller/router
// It will wipe the following directories and recreate them:./models, ./controllers, ./routers
// Newly geneated files will be inside these folders.
func writeSourceFiles(pkgPath string, tables []*Table, mode byte, paths *MvcPath) {
	if (OModel & mode) == OModel {
		beeLogger.Log.Info("Creating model files...")
		writeDTOModelFile(tables, paths.DTOPath)
		writeModelFiles(tables, paths.ModelPath)
	}
	if (OController & mode) == OController {
		beeLogger.Log.Info("Creating controller files...")
		writeControllerFiles(tables, paths.ControllerPath, pkgPath)
	}
	if (ORouter & mode) == ORouter {
		beeLogger.Log.Info("Creating router files...")
		writeRouterFile(tables, paths.RouterPath, pkgPath)
	}
}

// writeModelFiles 生成model文件
func writeModelFiles(tables []*Table, mPath string) {
	// 补充一个LgPager文件
	fpath := path.Join(mPath, "lg_pager.go")
	_ = ioutil.WriteFile(fpath, []byte(ModelLgPager), 0666)
	utils.FormatSourceCode(fpath)

	for _, tb := range tables {
		filename := getFileName(tb.Name)
		fpath := path.Join(mPath, filename+".go")
		var f *os.File
		var err error
		f, err = os.OpenFile(fpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
		if err != nil {
			beeLogger.Log.Warnf("%s", err)
			continue
		}

		var template string
		if tb.Pk == "" || strings.Contains(tb.Name, "_has_") {
			template = StructModelTPL
		} else {
			template = ModelTPL
		}

		// 关联的add
		everyRlAdd := "// every_rl  \n"
		for _, v := range tb.Columns {
			if v.Tag.M2M {
				everyRlAdd += "// m2m_add "
				everyRlAdd += fmt.Sprintf(`
					if m.%s != nil {
						if len(m.%s) != 0 {
							m2m := o.QueryM2M(m, "%s")
							_, err = m2m.Add(m.%s)
								if err != nil {
									o.Rollback()
									return
								}
						}
					}`, v.Name, v.Name, v.Name, v.Name) + "\n"
			} else if v.Tag.ReverseMany {
				everyRlAdd += "// o2m_add "
				everyRlAdd += fmt.Sprintf(`
					if m.%s != nil {
						if len(m.%s) != 0 {
							for i,_ := range m.%s {
								m.%s[i].{{modelName}} = &{{modelName}}{Id: m.Id}
							}
							_, err = o.InsertMulti(len(m.%s), m.%s)
							if err != nil {
								o.Rollback()
								return
							}
						}
					}`, v.Name, v.Name, v.Name, v.Name, v.Name, v.Name) + "\n"
			} else if v.Tag.ReverseOne {
				everyRlAdd += fmt.Sprintf(`
					if m.%s != nil {
						_, err = o.Insert(m.%s)
							if err != nil {
								o.Rollback()
								return
							}
					}`, v.Name, v.Name) + "\n"
			}
		}
		// 关联的update，先清空后加入
		every_rl_update := "// every_rl  \n"
		for _, v := range tb.Columns {
			if v.Tag.M2M {
				every_rl_update += "// m2m_update"
				every_rl_update += fmt.Sprintf(`
				if m.%s != nil {
				m2m := o.QueryM2M(m, "%s")
				_, err = m2m.Clear()
					if err != nil {
						o.Rollback()
						return
					}
					if len(m.%s) != 0 {
						_, err = m2m.Add(m.%s)
						if err != nil {
							o.Rollback()
							return
						}
					}
				}`, v.Name, v.Name, v.Name, v.Name) + "\n"
			} else if v.Tag.ReverseMany {
				every_rl_update += "// o2m_update"
				every_rl_update += fmt.Sprintf(`
				if m.%s != nil {
					if len(m.%s) != 0 {

					}
				}`, v.Name, v.Name) + "\n"
			}
		}
		// 关联的patch，需匹配字段才更新
		every_rl_patch := "// every_rl  \n"
		for _, v := range tb.Columns {
			if v.Tag.M2M {
				every_rl_patch += "// m2m_patch"
				every_rl_patch += fmt.Sprintf(`
				if fname == "%s" {
					if m.%s != nil {
					m2m := o.QueryM2M(m, "%s")
					_, err = m2m.Clear()
						if err != nil {
							o.Rollback()
							return
						}
					if len(m.%s) != 0 {
						_, err = m2m.Add(m.%s)
							if err != nil {
								o.Rollback()
								return
							}
						}
					}
					fields = append(fields[:index], fields[index+1:]...)
				}`, v.Name, v.Name, v.Name, v.Name, v.Name) + "\n"
			} else if v.Tag.ReverseMany {
				every_rl_patch += "// o2m_patch"
				every_rl_patch += fmt.Sprintf(`
				if fname == "%s" {
					if m.%s != nil {
						if len(m.%s) != 0 {

						}
					}
					fields = append(fields[:index], fields[index+1:]...)
				}`, v.Name, v.Name, v.Name) + "\n"
			}
		}
		// 关联的m2m，部分新增/删除
		every_m2m_part := "// every_m2m_part  \n"
		for _, v := range tb.Columns {
			if v.Tag.M2M {
				every_m2m_part += "// m2m_" + v.Name + v.Tag.Column
				every_m2m_part += fmt.Sprintf(`
				if field == "%s" {
					m2m := o.QueryM2M(m, "%s")
					if lenDel != 0 {
						for _, did := range DelIds {
							delone := &%s{Id: did}
							if m2m.Exist(delone) {
								_, err = m2m.Remove(delone)
								if err != nil {
									o.Rollback()
									return
								}
							}
						}
					}
					if lenAdd != 0 {
						for _, aid := range AddIds {
							addone := &%s{Id: aid}
							if !m2m.Exist(addone) {
								_, err = m2m.Add(addone)
								if err != nil {
									o.Rollback()
									return
								}
							}
						}
					}
				}`, v.Name, v.Name, strings.TrimRight(v.Name, "s"), strings.TrimRight(v.Name, "s")) + "\n"
			}
		}

		fileStr := strings.Replace(template, "{{modelStruct}}", tb.String(), 1)
		fileStr = strings.Replace(fileStr, "{{every_rl_add}}", everyRlAdd, -1)
		fileStr = strings.Replace(fileStr, "{{every_rl_update}}", every_rl_update, -1)
		fileStr = strings.Replace(fileStr, "{{every_rl_patch}}", every_rl_patch, -1)
		fileStr = strings.Replace(fileStr, "{{every_m2m_part}}", every_m2m_part, -1)
		fileStr = strings.Replace(fileStr, "{{modelName}}", utils.CamelCase(tb.Name), -1)
		fileStr = strings.Replace(fileStr, "{{tableName}}", tb.Name, -1)

		// 如果表包含时间字段，请导入time.Time包
		timePkg := ""
		importTimePkg := ""
		if tb.ImportTimePkg {
			timePkg = "\"time\"\n"
			importTimePkg = "import \"time\"\n"
		}
		fileStr = strings.Replace(fileStr, "{{timePkg}}", timePkg, -1)
		fileStr = strings.Replace(fileStr, "{{importTimePkg}}", importTimePkg, -1)
		if _, err := f.WriteString(fileStr); err != nil {
			beeLogger.Log.Fatalf("Could not write model file to '%s': %s", fpath, err)
		}
		utils.CloseFile(f)
		utils.FormatSourceCode(fpath)
	}
}

// writeDTOModelFile 生成TDO文件
func writeDTOModelFile(tables []*Table, mPath string) {
	filename := getFileName("dto_model")
	fpath := path.Join(mPath, filename+".go")

	fileStr := "package dto\nimport \"time\"\n"
	for _, tb := range tables {
		fileStr += tb.DTOString()
		fileStr += tb.JSONString()
	}

	_ = ioutil.WriteFile(fpath, []byte(fileStr), 0666)

	utils.FormatSourceCode(fpath)
}

// writeControllerFiles generates controller files
func writeControllerFiles(tables []*Table, cPath string, pkgPath string) {
	fpath := path.Join(cPath, "BaseController.go")

	// 只生成一次BaseController.go文件
	if !utils.IsExist(fpath) {
		_ = ioutil.WriteFile(fpath, []byte(BaseController), 0666)
		utils.FormatSourceCode(fpath)
	}

	for _, tb := range tables {
		if tb.Pk == "" || strings.Contains(tb.Name, "_has_") {
			continue
		}
		filename := getFileName(tb.Name)
		fpath := path.Join(cPath, filename+".go")
		var f *os.File
		var err error
		f, err = os.OpenFile(fpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
		if err != nil {
			beeLogger.Log.Warnf("%s", err)
			continue
		}

		colNamesString := "[]string{"
		for i, v := range tb.Columns {
			if !(!v.IsNeed || v.Tag.ReverseOne) {
				if i != 0 {
					colNamesString += ","
				}
				colNamesString += "\"" + v.Name + "\""
			}
		}
		colNamesString += "}"

		fileStr := strings.Replace(CtrlTPL, "{{ctrlName}}", utils.CamelCase(tb.Name), -1)
		description := strings.Replace(tb.Comments, "表", "", -1)
		if len(description) <= 0 {
			description = tb.Name
		}
		fileStr = strings.Replace(fileStr, "{{Description}}", description, -1)
		fileStr = strings.Replace(fileStr, "{{pkgPath}}", pkgPath, -1)
		fileStr = strings.Replace(fileStr, "{{colNamesString}}", colNamesString, -1)
		if _, err := f.WriteString(fileStr); err != nil {
			beeLogger.Log.Fatalf("Could not write controller file to '%s': %s", fpath, err)
		}
		utils.CloseFile(f)
		utils.FormatSourceCode(fpath)
	}
}

// writeRouterFile generates router file
func writeRouterFile(tables []*Table, rPath string, pkgPath string) {
	var nameSpaces []string
	for _, tb := range tables {
		if tb.Pk == "" || strings.Contains(tb.Name, "_has_") {
			continue
		}
		// Add namespaces
		nameSpace := strings.Replace(NamespaceTPL, "{{nameSpace}}", tb.Name, -1)
		nameSpace = strings.Replace(nameSpace, "{{ctrlName}}", utils.CamelCase(tb.Name), -1)
		nameSpaces = append(nameSpaces, nameSpace)
	}

	// Add export controller
	fpath := filepath.Join(rPath, "router.go")
	routerStr := strings.Replace(RouterTPL, "{{nameSpaces}}", strings.Join(nameSpaces, ""), 1)
	routerStr = strings.Replace(routerStr, "{{pkgPath}}", pkgPath, 1)
	var f *os.File
	var err error
	f, err = os.OpenFile(fpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		beeLogger.Log.Warnf("%s", err)
		return
	}
	if _, err := f.WriteString(routerStr); err != nil {
		beeLogger.Log.Fatalf("Could not write router file to '%s': %s", fpath, err)
	}
	utils.CloseFile(f)
	utils.FormatSourceCode(fpath)
}

func isSQLTemporalType(t string) bool {
	return t == "date" || t == "datetime" || t == "timestamp" || t == "time"
}

func isSQLStringType(t string) bool {
	return t == "char" || t == "varchar"
}

func isSQLSignedIntType(t string) bool {
	return t == "int" || t == "tinyint" || t == "smallint" || t == "mediumint" || t == "bigint"
}

func isSQLDecimal(t string) bool {
	return t == "decimal"
}

func isSQLBinaryType(t string) bool {
	return t == "binary" || t == "varbinary"
}

func isSQLBitType(t string) bool {
	return t == "bit"
}

// extractColSize 提取字段大小：例如varchar（255）=> 255
func extractColSize(colType string) string {
	regex := regexp.MustCompile(`^[a-z]+\(([0-9]+)\)$`)
	size := regex.FindStringSubmatch(colType)
	return size[1]
}

func extractIntSignness(colType string) string {
	regex := regexp.MustCompile(`(int|smallint|mediumint|bigint)\([0-9]+\)(.*)`)
	signRegex := regex.FindStringSubmatch(colType)
	return strings.Trim(signRegex[2], " ")
}

// extractDecimal 提取小数部分
func extractDecimal(colType string) (digits string, decimals string) {
	decimalRegex := regexp.MustCompile(`decimal\(([0-9]+),([0-9]+)\)`)
	decimal := decimalRegex.FindStringSubmatch(colType)
	digits, decimals = decimal[1], decimal[2]
	return
}

// getFileName 获取文件名，主要是为了处理_test的文件名
func getFileName(tbName string) (filename string) {
	// avoid test file
	filename = tbName
	for strings.HasSuffix(filename, "_test") {
		pos := strings.LastIndex(filename, "_")
		filename = filename[:pos] + filename[pos+1:]
	}
	return
}

// getPackagePath 获取包路径
func getPackagePath(curpath string) (packpath string) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		beeLogger.Log.Fatal("GOPATH environment variable is not set or empty")
	}

	beeLogger.Log.Debugf("GOPATH: %s", utils.FILE(), utils.LINE(), gopath)

	appsrcpath := ""
	haspath := false
	wgopath := filepath.SplitList(gopath)

	for _, wg := range wgopath {
		wg, _ = filepath.EvalSymlinks(filepath.Join(wg, "src"))
		if strings.HasPrefix(strings.ToLower(curpath), strings.ToLower(wg)) {
			haspath = true
			appsrcpath = wg
			break
		}
	}

	if !haspath {
		beeLogger.Log.Fatalf("Cannot generate application code outside of GOPATH '%s' compare with CWD '%s'", gopath, curpath)
	}

	if curpath == appsrcpath {
		beeLogger.Log.Fatal("Cannot generate application code outside of application path")
	}

	packpath = strings.Join(strings.Split(curpath[len(appsrcpath)+1:], string(filepath.Separator)), "/")
	return
}

const (
	// 多对多的model模板
	StructModelTPL = `package models
{{importTimePkg}}
{{modelStruct}}	
`
	// 分页的设计
	ModelLgPager = `package models

	type LgPage struct {
		PageNo     int64
		PageSize   int64
		TotalPage  int64
		TotalCount int64
		FirstPage  bool
		LastPage   bool
	}
	
	type LgPager struct {
		Page LgPage
		List interface{}
	}
	
	func (p *LgPager) PageUtil(count int64, pageNo int64, pageSize int64) LgPage {
		tp := count / pageSize
		if count%pageSize > 0 {
			tp = count/pageSize + 1
		}
		return LgPage{PageNo: pageNo, PageSize: pageSize, TotalPage: tp, TotalCount: count, FirstPage: pageNo == 1, LastPage: pageNo == tp}
	}
	`
	// model模板
	ModelTPL = `package models

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	{{timePkg}}
	"github.com/astaxie/beego/orm"
)

{{modelStruct}}

func (t *{{modelName}}) TableName() string {
	return "{{tableName}}"
}

func init() {
	orm.RegisterModel(new({{modelName}}))
}

func (t *{{modelName}}) LoadRelatedOf(r string, args ...interface{}) (int64, error) {
	o := orm.NewOrm()
	num, err := o.LoadRelated(t, r, args)
	return num, err
}

// Add{{modelName}} insert a new {{modelName}} into database and returns
// last inserted Id on success.
func Add{{modelName}}(m *{{modelName}}) (id int64, err error) {
	o := orm.NewOrm()
	id, err = o.Insert(m)
	return
}

// AddMulti{{modelName}} insert multi {{modelName}}s into database and returns
// sum success nums.
func AddMulti{{modelName}}(ms []*{{modelName}}) (successNums int64, err error) {
	o := orm.NewOrm()
	if len(ms) != 0 {
		successNums, err = o.InsertMulti(len(ms), ms)
		if err != nil {
			return
		}
	} else {
		successNums = 0
	}
	return
}

// Add{{modelName}}HasMany insert a new {{modelName}} and some items into database and returns
// last inserted Id on success.
func Add{{modelName}}HasMany(m *{{modelName}}) (id int64, err error) {
	o := orm.NewOrm()
	o.Begin()
	id, err = o.Insert(m)
	if err != nil {
		o.Rollback()
		return
	}
	m.Id = int(id)

	{{every_rl_add}}

	o.Commit()
	return
}

// Get{{modelName}}ById retrieves {{modelName}} by Id. Returns error if
// Id doesn't exist
func Get{{modelName}}ById(id int) (v *{{modelName}}, err error) {
	o := orm.NewOrm()
	v = &{{modelName}}{Id: id}
	if err = o.Read(v); err == nil {
		return v, nil
	}
	return nil, err
}

// Get{{modelName}}Counts retrieves counts matches certain condition. Returns empty list if
// no records exist
func Get{{modelName}}Counts(query map[string]string) (count int64, err error) {
	o := orm.NewOrm()
	qs := o.QueryTable(new({{modelName}}))
	// query k=v
	for k, v := range query {
		// rewrite dot-notation to Object__Attribute
		k = strings.Replace(k, ".", "__", -1)
		if strings.Contains(k, "isnull") {
			qs = qs.Filter(k, (v == "true" || v == "1"))
		} else {
			qs = qs.Filter(k, v)
		}
	}
	count, err = qs.Count()
	return count, err
}

// GetAll{{modelName}} retrieves all {{modelName}} matches certain condition. Returns empty list if
// no records exist
func GetAll{{modelName}}(query map[string]string, fields []string, sortby []string, order []string,
	offset int64, limit int64, load []string, page int64) (ml []interface{},pager *LgPager, err error) {
	o := orm.NewOrm()
	qs := o.QueryTable(new({{modelName}}))
	var count int64 = 0
	// query k=v
	if len(query) > 0 {
		cond := orm.NewCondition()
		search_arr_str := ""
		dsearch_arr_str := ""
		var co_arr []*orm.Condition
		for k, v := range query {
			v = strings.Replace(v, ".", "__", -1)
			switch k {
			//1.非空,示例:not_empty:column
			case "not_empty":
				cond1 := cond.And(v+"__isnull", false).AndNot(v, "")
				co_arr = append(co_arr, cond1)
			//2.或搜索,示例:search:column1>value1|column2>value2
			case "search":
				search_arr_str = v
			case "dsearch":
				dsearch_arr_str = v
			//3.不等于,示例:neq:column1>value1
			case "neq":
				filed := strings.Split(v, ">")
				if len(filed) == 2 {
					filed[0] = strings.Replace(filed[0], ".", "__", -1)
					cond1 := cond.AndNot(filed[0], filed[1])
					co_arr = append(co_arr, cond1)
				}
			default:
				k = strings.Replace(k, ".", "__", -1)
				if strings.Contains(k, "isnull") {
					cond1 := cond.And(k, (v == "true" || v == "1"))
					co_arr = append(co_arr, cond1)
				} else {
					cond2 := cond.And(k, v)
					co_arr = append(co_arr, cond2)
				}
			}
		}

		search_arr := strings.Split(search_arr_str, "^")
		if len(search_arr) > 0 {
			for _, v := range search_arr {
				searchFields := strings.Split(v, "|")
				var co1 *orm.Condition
				for _, item := range searchFields {
					filed := strings.Split(item, ">")
					if len(filed) == 2 {
						key := filed[0] + "__contains"
						value := filed[1]
						if co1 == nil {
							tmp := cond.And(key, value)
							co1 = tmp
						} else {
							co1 = co1.Or(key, value)
						}
					}
				}
				co_arr = append(co_arr, co1)
			}
		}

		dsearch_arr := strings.Split(dsearch_arr_str, "^")
		if len(dsearch_arr) > 0 {
			for _, v := range dsearch_arr {
				searchFields := strings.Split(v, "|")
				var co1 *orm.Condition
				for _, item := range searchFields {
					filed := strings.Split(item, ">")
					if len(filed) == 2 {
						key := filed[0]
						value := filed[1]
						if co1 == nil {
							tmp := cond.And(key, value)
							co1 = tmp
						} else {
							co1 = co1.Or(key, value)
						}
					}
				}
				co_arr = append(co_arr, co1)
			}
		}

		if len(co_arr) > 0 {
			var co2 *orm.Condition
			for _, item := range co_arr {
				if co2 == nil {
					tmp := cond.AndCond(item)
					co2 = tmp
				} else {
					co2 = co2.AndCond(item)
				}
			}
			qs = qs.SetCond(co2)
		}

	}
	// order by:
	var sortFields []string
	if len(sortby) != 0 {
		if len(sortby) == len(order) {
			// 1) for each sort field, there is an associated order
			for i, v := range sortby {
				orderby := ""
				if order[i] == "desc" {
					orderby = "-" + v
				} else if order[i] == "asc" {
					orderby = v
				} else {
					return nil,nil, errors.New("Error: Invalid order. Must be either [asc|desc]")
				}
				sortFields = append(sortFields, orderby)
			}
			qs = qs.OrderBy(sortFields...)
		} else if len(sortby) != len(order) && len(order) == 1 {
			// 2) there is exactly one order, all the sorted fields will be sorted by this order
			for _, v := range sortby {
				orderby := ""
				if order[0] == "desc" {
					orderby = "-" + v
				} else if order[0] == "asc" {
					orderby = v
				} else {
					return nil,nil, errors.New("Error: Invalid order. Must be either [asc|desc]")
				}
				sortFields = append(sortFields, orderby)
			}
		} else if len(sortby) != len(order) && len(order) != 1 {
			return nil,nil, errors.New("Error: 'sortby', 'order' sizes mismatch or 'order' size is not 1")
		}
	} else {
		if len(order) != 0 {
			return nil,nil, errors.New("Error: unused 'order' fields")
		}
	}

	var l []{{modelName}}
	qs = qs.OrderBy(sortFields...)

	if page == 1 {
		count,_ = qs.Count()
	}


	if _, err = qs.Limit(limit, offset).All(&l, fields...); err == nil {
		if len(fields) == 0 {
			for _, v := range l {
				for _, lo := range load {
					v.LoadRelatedOf(lo)
				}
				ml = append(ml, v)
			}
		} else {
			// trim unused fields
			for _, v := range l {
				m := make(map[string]interface{})
				val := reflect.ValueOf(v)
				for _, fname := range fields {
					m[fname] = val.FieldByName(fname).Interface()
				}
				for _, lo := range load {
					v.LoadRelatedOf(lo)
				}
				ml = append(ml, m)
			}
		}
		
		if len(ml) == 0 {
			ml = make([]interface{}, 0)
		}

		if page == 1 {
			pager = &LgPager{}
			pager.Page = pager.PageUtil(count, offset/limit + 1, limit)
			pager.List = ml
			return ml,pager, nil
		} else {
			return ml,nil, nil
		}

		
	}
	return nil,nil, err
}

// Update{{modelName}} updates {{modelName}} by Id and returns error if
// the record to be updated doesn't exist
func Update{{modelName}}ById(m *{{modelName}}) (err error) {
	o := orm.NewOrm()
	o.Begin()
	v := {{modelName}}{Id: m.Id}
	
	{{every_rl_update}}

	// ascertain id exists in the database
	if err = o.Read(&v); err == nil {
		_, err = o.Update(m)
		if err != nil {
			o.Rollback()
			return
		}
	}

	o.Commit()
	return
}

// Patch{{modelName}} updates {{modelName}} by Id and returns error if
// the record to be updated doesn't exist
func Patch{{modelName}}ById(m *{{modelName}}, fields []string) (err error) {
	o := orm.NewOrm()
	o.Begin()
	
	for index, fname := range fields {
		if fname == "" {
			continue
		}
		if index == -1 {
			continue
		}
		{{every_rl_patch}}
	}
	
	_, err = o.Update(m, fields...)
	if err != nil {
		o.Rollback()
		return
	}
	o.Commit()
	return
}

// Patch{{modelName}}M2MPart updates {{modelName}} by Id and returns error if
// the record to be updated doesn't exist
func Patch{{modelName}}M2MPartById(m *{{modelName}}, field string, AddIds, DelIds []int) (err error) {
	lenDel := len(DelIds)
	lenAdd := len(AddIds)
	if lenDel == 0 && lenAdd == 0 {
		err = errors.New("Add和Del不能同时为[]！")
		return
	}
	o := orm.NewOrm()
	o.Begin()

	{{every_m2m_part}}
	
	o.Commit()
	return
}

// Delete{{modelName}} deletes {{modelName}} by Id and returns error if
// the record to be deleted doesn't exist
func Delete{{modelName}}(id int) (err error) {
	o := orm.NewOrm()
	v := {{modelName}}{Id: id}
	// ascertain id exists in the database
	if err = o.Read(&v); err == nil {
		var num int64
		if num, err = o.Delete(&{{modelName}}{Id: id}); err == nil {
			fmt.Println("Number of records deleted in database:", num)
		}
	}
	return
}
`
	// controller模板
	CtrlTPL = `package controllers

import (
	"{{pkgPath}}/models"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"github.com/tidwall/gjson"
	// posimport
)

// {{ctrlName}}Controller operations for {{ctrlName}}
type {{ctrlName}}Controller struct {
	BaseController
}

// URLMapping ...
func (c *{{ctrlName}}Controller) URLMapping() {
	c.Mapping("Post", c.Post)
	c.Mapping("GetOne", c.GetOne)
	c.Mapping("GetAll", c.GetAll)
	c.Mapping("Put", c.Put)
	c.Mapping("Patch", c.Patch)
	c.Mapping("PatchM2MPart", c.PatchM2MPart)
	c.Mapping("Delete", c.Delete)
}

// @Description 新建{{Description}}
// @router / [post]
func (c *{{ctrlName}}Controller) Post() {
	// pos11
	jr := gjson.ParseBytes(c.Ctx.Input.RequestBody)
	if jr.IsObject() {
		var v models.{{ctrlName}}
		if err := json.Unmarshal(c.Ctx.Input.RequestBody, &v); err == nil {
			// pos12
			if _, err := models.Add{{ctrlName}}HasMany(&v); err == nil {
				// pos13
				c.Ctx.Output.SetStatus(201)
				c.Data["json"] = v
			} else {
				c.Ctx.Output.SetStatus(400)
				c.Data["json"] = err.Error()
			}
		} else {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		}
	} else {
		var vs []*models.{{ctrlName}}
		if err := json.Unmarshal(c.Ctx.Input.RequestBody, &vs); err == nil {
			if successNums, err := models.AddMulti{{ctrlName}}(vs); err == nil {
				c.Ctx.Output.SetStatus(201)
				c.Data["json"] = successNums
			} else {
				c.Ctx.Output.SetStatus(400)
				c.Data["json"] = err.Error()
			}
		} else {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		}
	}
	c.ServeJSON()
}

// @Description 获取{{Description}}信息
// @router /:id [get]
func (c *{{ctrlName}}Controller) GetOne() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	v, err := models.Get{{ctrlName}}ById(id)
	var load []string

	if v := c.GetString("load"); v != "" {
		load = strings.Split(v, ",")
	}
	// pos21
	if err != nil {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	} else {
	// pos22
	if len(load) != 0 {
			for _, lo := range load {
				_, err := v.LoadRelatedOf(lo)
				if err != nil {
					c.Ctx.Output.SetStatus(400)
					c.Data["json"] = err.Error()
					c.ServeJSON()
					return
				}
			}
		}
		c.Data["json"] = v
	}
	c.ServeJSON()
}

// GetAll ...
// @Title Get All
// @Description 搜索{{Description}}信息
// @Param	query	query	string	false	"Filter. e.g. col1:v1,col2:v2 ..."
// @Param	fields	query	string	false	"Fields returned. e.g. col1,col2 ..."
// @Param	sortby	query	string	false	"Sorted-by fields. e.g. col1,col2 ..."
// @Param	order	query	string	false	"Order corresponding to each sortby field, if single value, apply to all sortby fields. e.g. desc,asc ..."
// @Param	limit	query	string	false	"Limit the size of result set. Must be an integer"
// @Param	offset	query	string	false	"Start position of result set. Must be an integer"
// @Param	page	query	string	false	"Page number of result set. Must be an integer"
// @Param	load	query	string	false	"LoadRelatedOf. e.g. As,Bs,C ..."
// @Param	getcounts	query	int	false	"GetCounts. e.g. 传1时仅返回记录数"
// @Success 200 {object} models.{{ctrlName}}
// @Failure 403
// @router / [get]
func (c *{{ctrlName}}Controller) GetAll() {
	var fields []string
	var sortby []string
	var order []string
	var load []string
	var query = make(map[string]string)
	var limit int64 = 10
	var page int64 = 0
	var offset int64
	var getcounts int = 0

	// getcounts: 0 (default is 0)
	if v, err := c.GetInt("getcounts"); err == nil {
		getcounts = v
	}

	// query: k:v,k:v
	if v := c.GetString("query"); v != "" {
		for _, cond := range strings.Split(v, ",") {
			kv := strings.SplitN(cond, ":", 2)
			if len(kv) != 2 {
				c.Data["json"] = errors.New("Error: invalid query key/value pair")
				c.ServeJSON()
				return
			}
			k, v := kv[0], kv[1]
			query[k] = v
		}
	}

	if getcounts == 1 {
		nums, err := models.Get{{ctrlName}}Counts(query)
		if err != nil {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		} else {
			c.Data["json"] = nums
		}
		c.ServeJSON()
		return
	}

	// fields: col1,col2,entity.col3
	if v := c.GetString("fields"); v != "" {
		fields = strings.Split(v, ",")
	}
	if v := c.GetString("load"); v != "" {
		load = strings.Split(v, ",")
	}

	// order: desc,asc
	if v := c.GetString("order"); v != "" {
		order = strings.Split(v, ",")
	}
	// sortby: col1,col2
	if v := c.GetString("sortby"); v != "" {
		sortby = strings.Split(v, ",")
	}
	if v, err := c.GetInt64("page"); err == nil {
		page = v
	}
	// limit: 10 (default is 10)
	if v, err := c.GetInt64("limit"); err == nil {
		limit = v
	}
	// offset: 0 (default is 0)
	if v, err := c.GetInt64("offset"); err == nil {
		offset = v
	}

	l, pager, err := models.GetAll{{ctrlName}}(query, fields, sortby, order, offset, limit, load, page)
	// pos31
	if err != nil {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	} else {
		if pager != nil {
			c.Data["json"] = pager
		} else {
			c.Data["json"] = l
		}
	}
	c.ServeJSON()
}

// @Description 修改{{Description}}
// @router /:id [put]
func (c *{{ctrlName}}Controller) Put() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	v := models.{{ctrlName}}{Id: id}

	raw := string(c.Ctx.Input.RequestBody)
	fileds := []string{}
	oriFields := {{colNamesString}}
	
	for _, oriFiled:= range oriFields {
		re := regexp.MustCompile("\"" + oriFiled + "\":")
		match := re.FindAllString(raw, -1)
		if len(match) > 0 {
			fileds = append(fileds, oriFiled)
		} 
	}
	if len(fileds) == 0 {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = "没有匹配字段！"
		c.ServeJSON()
		return
	}

	// pos41
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &v); err == nil {
		// pos42
		if err := models.Patch{{ctrlName}}ById(&v, fileds); err == nil {
			// pos43
			c.Data["json"] = "OK"
		} else {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		}
	} else {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	}
	c.ServeJSON()
}

// @Description 修改{{Description}}
// @router /:id [Patch]
func (c *{{ctrlName}}Controller) Patch() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	v := models.{{ctrlName}}{Id: id}

	raw := string(c.Ctx.Input.RequestBody)
	fileds := []string{}
	oriFields := {{colNamesString}}
	
	for _, oriFiled:= range oriFields {
		re := regexp.MustCompile("\"" + oriFiled + "\":")
		match := re.FindAllString(raw, -1)
		if len(match) > 0 {
			fileds = append(fileds, oriFiled)
		} 
	}
	if len(fileds) == 0 {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = "没有匹配字段！"
		c.ServeJSON()
		return
	}

	// pos51
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &v); err == nil {
		// pos52
		if err := models.Patch{{ctrlName}}ById(&v, fileds); err == nil {
			// pos53
			c.Data["json"] = "OK"
		} else {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		}
	} else {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	}
	c.ServeJSON()
}

// @Description 修改{{Description}}的关系
// @router /m2m/part/:id [Patch]
func (c *{{ctrlName}}Controller) PatchM2MPart() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	v := models.{{ctrlName}}{Id: id}

	var m2mField string
	// field
	if v := c.GetString("m2m_field"); v != "" {
		m2mField = v
	} else {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = "m2m_field不能为空！"
		c.ServeJSON()
		return
	}
	AddOrDelIds := struct {
		Add []int
		Del []int
	}{}

	// pos_m2m_1
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &AddOrDelIds); err == nil {
		// pos_m2m_2
		if err := models.Patch{{ctrlName}}M2MPartById(&v, m2mField, AddOrDelIds.Add, AddOrDelIds.Del); err == nil {
		// pos_m2m_3
			c.Data["json"] = "OK"
		} else {
			c.Ctx.Output.SetStatus(400)
			c.Data["json"] = err.Error()
		}
	} else {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	}
	c.ServeJSON()
}

// @Description 删除{{Description}}
// @router /:id [delete]
func (c *{{ctrlName}}Controller) Delete() {
	idStr := c.Ctx.Input.Param(":id")
	id, _ := strconv.Atoi(idStr)
	if err := models.Delete{{ctrlName}}(id); err == nil {
		c.Data["json"] = "OK"
	} else {
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = err.Error()
	}
	c.ServeJSON()
}
`
	// router模板
	RouterTPL = `// @APIVersion 1.0.0
// @Title beego Test API
// @Description beego has a very cool tools to autogenerate documents for your API
// @Contact astaxie@gmail.com
// @TermsOfServiceUrl http://beego.me/
// @License Apache 2.0
// @LicenseUrl http://www.apache.org/licenses/LICENSE-2.0.html
package routers

import (
	"{{pkgPath}}/controllers"

	"github.com/astaxie/beego"
)

func init() {
	ns := beego.NewNamespace("/api",
		{{nameSpaces}}

		// posrouter
	)
	beego.AddNamespace(ns)
}
`
	// routerNamespace模板
	NamespaceTPL = `
		beego.NSNamespace("/{{nameSpace}}",
			beego.NSInclude(
				&controllers.{{ctrlName}}Controller{},
			),
		),
`
	// 自定义的的Controller模板
	BaseController = `package controllers

	import (
		"crypto/md5"
		"crypto/tls"
		"encoding/hex"
		"encoding/json"
		"errors"
		"fmt"
		"io/ioutil"
		"net/http"
		"net/url"
		"os"
		"sort"
		"strconv"
		"strings"
		"time"
	
		"github.com/astaxie/beego"
		"github.com/astaxie/beego/context"
		"github.com/astaxie/beego/httplib"
		"github.com/dgrijalva/jwt-go"
	)
	
	var (
		JWT_PUBLIC_KEY                  []byte
		appkey, appsecret, accessSecret string
		openApiSign                     bool   = false
		openJwt                         bool   = false
		openPerm                        bool   = false
		CENTER_SERVICE                  string = beego.AppConfig.String("center_service")
	)
	
	func init() {
		appkey = beego.AppConfig.String("Appkey")
		appsecret = beego.AppConfig.String("Appsecret")
		// accessSecret用于签名
		accessSecret = beego.AppConfig.String("AccessSecret")
		// 三个开关，分别是 API签名验证、JWT合法性验证及解析、路由权限验证
		openApiSign, _ = beego.AppConfig.Bool("open_api_sign")
		openJwt, _ = beego.AppConfig.Bool("open_jwt")
		openPerm, _ = beego.AppConfig.Bool("open_perm")
		// 当启用JWT时，才读取公钥
		if openJwt {
			f, err := os.Open("keys/jwt_public_key.pem")
			if err != nil {
				panic(err)
			}
			defer f.Close()
	
			fd, err := ioutil.ReadAll(f)
			if err != nil {
				panic(err)
			}
			JWT_PUBLIC_KEY = fd
		}
	
		// 拦截器，拦截所有路由
		beego.InsertFilter("/*", beego.BeforeExec, FilterRouter, true, false)
	}
	
	// Ignored FilterToken
	var ignoredTokenRouter = map[string]bool{
		"post@/api/user/login":       true,
		"post@/api/user/login/oauth": true,
	}
	
	// Ignored PermRouter
	var ignoredPermRouter = map[string]bool{
		"post@/api/user/login":         true,
		"post@/api/user/login/refresh": true,
		"post@/api/user/login/oauth":   true,
	}
	
	type BaseController struct {
		beego.Controller
	}
	
	// JsonResult 用于返回ajax请求的基类
	type JsonResult struct {
		Code    int
		Message string
	}
	
	type JWTInfo struct {
		Token    string
		ExpireAt int64
	}
	
	//返回json结果，并中断
	func (c *BaseController) jsonResult(code int, msg string, data interface{}) {
		r := &JsonResult{Code: code, Message: msg}
		c.Ctx.Output.SetStatus(400)
		c.Data["json"] = map[string]interface{}{"Result": r, "Data": data}
		c.ServeJSON()
		c.StopRun()
	}
	
	//返回json更多结果，并中断
	func (c *BaseController) jsonResultMore(code int, msg string, data interface{}, m interface{}) {
		r := &JsonResult{Code: code, Message: msg}
		c.Data["json"] = map[string]interface{}{"Result": r, "Data": data, "More": m}
		c.ServeJSON()
		c.StopRun()
	}
	
	//返回json分页结果，并中断
	func (c *BaseController) jsonResultByPage(code int, msg string, data interface{}, p interface{}) {
		r := &JsonResult{Code: code, Message: msg}
		c.Data["json"] = map[string]interface{}{"Result": r, "Data": data, "Page": p}
		c.ServeJSON()
		c.StopRun()
	}
	
	//返回json结果，设置状态码，并中断
	func (c *BaseController) jsonResponse(code int, msg string, data interface{}) {
		c.Ctx.Output.SetStatus(code)
		c.Data["json"] = map[string]interface{}{"Data": data, "Msg": msg}
		c.ServeJSON()
		c.StopRun()
	}
	
	// 获取请求中的JWT字符串
	func (base *BaseController) GetAccessToken() string {
		actData := base.Ctx.Input.GetData("JWTToken")
		act, ok := actData.(string)
		if ok {
			return act
		}
		return ""
	}
	
	// Parse JWTClaims in Ctx.Data["JWTClaims"]
	func (base *BaseController) ParseClaims() map[string]interface{} {
		cl := base.Ctx.Input.GetData("JWTClaims")
		if cl != nil {
			clmap, ok := cl.(map[string]interface{})
			if ok {
				return clmap
			}
			return nil
		}
		return nil
	}
	
	// Parse JWTClaims in Ctx.Data["JWTClaims"]
	func parseClaims(ctx *context.Context) map[string]interface{} {
		cl := ctx.Input.GetData("JWTClaims")
		if cl != nil {
			clmap, ok := cl.(map[string]interface{})
			if ok {
				return clmap
			}
			return nil
		}
		return nil
	}
	
	// Recover Route
	func RecoverRoute(ctx *context.Context) string {
		route := strings.Split(ctx.Request.URL.RequestURI(), "?")[0]
		// 将路径中的参数值替换为参数名
		for k, v := range ctx.Input.Params() {
			// 如果参数是 :splat等预定义的，则跳过
			if k == ":splat" || k == ":path" || k == ":ext" {
				continue
			}
			route = strings.Replace(route, "/"+v, "/"+k, 1)
		}
		// 路径格式均为 请求类型@路径
		route = strings.ToLower(ctx.Request.Method) + "@" + route
		return route
	}
	
	// 路由拦截器的Filter
	var FilterRouter = func(ctx *context.Context) {
		if openApiSign {
			signOk := VerifySign(ctx)
			if !signOk {
				return
			}
		}
		if openJwt {
			// 路径格式均为 请求类型@路径
			route := RecoverRoute(ctx)
			// 直接通过map查询是否忽略
			if _, ok := ignoredTokenRouter[route]; ok {
				return
			}
			// 验证JWT是否有效
			jwtOk := VerifyToken(ctx)
			if jwtOk {
				if openPerm {
					// 直接通过map查询是否忽略
					if _, ok := ignoredPermRouter[route]; ok {
						return
					}
					// todo:或通过缓存查询是否已有权限查询记录
					// 如果没有，向聚合平台查询，并缓存
					permOk := VerifyPerm(route, ctx)
					if permOk {
						return
					}
				}
			} else {
				return
			}
		}
	}
	
	func VerifyToken(ctx *context.Context) bool {
		authString := ctx.Input.Header("Authorization")
		kv := strings.Split(authString, " ")
		if len(kv) != 2 || kv[0] != "Bearer" {
			beego.Error("Authorization格式不对或Token为空！")
			http.Error(ctx.ResponseWriter, "Authorization格式不对或Token为空！", http.StatusUnauthorized)
			return false
		}
		tokenString := kv[1]
		ctx.Input.SetData("JWTToken", tokenString)
	
		// Parse token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// 必要的验证 RS256
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			//// 可选项验证  'aud' claim
			//aud := "https://api.cn.atomintl.com"
			//checkAud := token.Claims.(jwt.MapClaims).VerifyAudience(aud, false)
			//if !checkAud {
			//  return token, errors.New("Invalid audience.")
			//}
			// 必要的验证 'iss' claim
			iss := "https://atomintl.auth0.com/"
			checkIss := token.Claims.(jwt.MapClaims).VerifyIssuer(iss, false)
			if !checkIss {
				return token, errors.New("Invalid issuer.")
			}
	
			result, _ := jwt.ParseRSAPublicKeyFromPEM(JWT_PUBLIC_KEY)
			//result := []byte(cert) // 不是正确的 PUBKEY 格式 都会 报  key is of invalid type
			return result, nil
		})
		if err != nil {
			beego.Error("Parse token error:", err)
			if ve, ok := err.(*jwt.ValidationError); ok {
				if ve.Errors&jwt.ValidationErrorMalformed != 0 {
					// That's not even a token
					http.Error(ctx.ResponseWriter, "Token 格式有误！", http.StatusUnauthorized)
					return false
				} else if ve.Errors&(jwt.ValidationErrorExpired|jwt.ValidationErrorNotValidYet) != 0 {
					// Token is either expired or not active yet
					http.Error(ctx.ResponseWriter, "Token 已过期！", http.StatusUnauthorized)
					return false
				} else {
					// Couldn't handle this token
					http.Error(ctx.ResponseWriter, "验证Token的过程中发生其他错误！", http.StatusUnauthorized)
					return false
				}
			} else {
				// Couldn't handle this token
				http.Error(ctx.ResponseWriter, "无法处理此Token！", http.StatusUnauthorized)
				return false
			}
		}
		if !token.Valid {
			beego.Error("Token invalid:", tokenString)
			http.Error(ctx.ResponseWriter, "Token 不合法:"+tokenString, http.StatusUnauthorized)
			return false
		}
		// beego.Debug("Token:", token)
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			beego.Error("转换为jwt.MapClaims失败")
			return false
		}
		var claimsMIF = make(map[string]interface{})
		jsonM, _ := json.Marshal(&claims)
		json.Unmarshal(jsonM, &claimsMIF)
		ctx.Input.SetData("JWTClaims", claimsMIF)
		return true
	}
	
	func VerifyPerm(route string, ctx *context.Context) bool {
		var subT, subV string
		cls := parseClaims(ctx)
		if cls != nil {
			subT = cls["sub_type"].(string)
			subV = cls["sub_value"].(string)
		}
		if subT == "" {
			http.Error(ctx.ResponseWriter, "JWT中的SubType不能为空！", http.StatusUnauthorized)
			return false
		}
		v := &struct {
			SubType  string
			SubValue string
			Perm     string
			Ops      string
			AppId    string
		}{}
		v.SubType = subT
		v.SubValue = subV
		v.Perm = route
		v.Ops = "999"
		req := httplib.Post(CENTER_SERVICE + "/rule_perm/check")
		req.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		jsonM, _ := json.Marshal(v)
		req.Body(jsonM)
		resp, err := req.Response()
		if err == nil {
			if resp.StatusCode == 200 {
				return true
			} else {
				http.Error(ctx.ResponseWriter, "没有权限！", http.StatusUnauthorized)
				return false
			}
		} else {
			http.Error(ctx.ResponseWriter, "请求权限检查出错！", http.StatusUnauthorized)
			return false
		}
	}
	
	// 验证签名
	func VerifySign(c *context.Context) bool {
		_ = c.Request.ParseForm()
		req := c.Request.Form
		// bd := c.Input.CopyBody(1048576)
		bd := c.Input.RequestBody
		var app_key, sn, ts string
	
		if v := c.Request.FormValue("app_key"); v != "" {
			app_key = v
		}
		if v := c.Request.FormValue("sn"); v != "" {
			sn = v
		}
		if v := c.Request.FormValue("ts"); v != "" {
			ts = v
		}
	
		// 判断app_key
		if app_key == "" || app_key != appkey {
			http.Error(c.ResponseWriter, "app_key错误，请核对提交应用key!", http.StatusForbidden)
			return false
		}
	
		// 验证过期时间
		timestamp := time.Now().Unix()
		exp := int64(600)
		tsInt, _ := strconv.ParseInt(ts, 10, 64)
		if tsInt > timestamp || timestamp-tsInt >= exp {
			http.Error(c.ResponseWriter, "ts错误，请求已过期!", http.StatusForbidden)
			return false
		}
	
		beego.Debug("调试sign值：", createSignMD5(req, bd, accessSecret))
		// 验证签名
		if sn == "" || sn != createSignMD5(req, bd, accessSecret) {
			http.Error(c.ResponseWriter, "sn错误，请核对签名!", http.StatusForbidden)
			return false
		}
		return true
	}
	
	func (base *BaseController) GetAppRoleToken(roleCode, majorParms string) (*JWTInfo, error) {
		var rjwt JWTInfo
		v := &struct {
			RoleCode   string
			AppId      string
			MajorParms string
		}{}
		v.AppId = appkey
		v.RoleCode = roleCode
		v.MajorParms = majorParms
		req := httplib.Post(CENTER_SERVICE + "/rule_auth/login/app_role")
		req.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		jsonM, _ := json.Marshal(v)
		req.Body(jsonM)
		resp, err := req.Response()
		if err == nil {
			if resp.StatusCode == 200 {
				err := req.ToJSON(&rjwt)
				if err != nil {
					return nil, err
				}
				return &rjwt, nil
			} else {
				msgResult, _ := req.String()
				return nil, errors.New(msgResult)
			}
		} else {
			return nil, err
		}
	}
	
	// 创建MD5签名
	func createSignMD5(params url.Values, body []byte, AS string) string {
		// 自定义 MD5 组合
		return EncodeStrMd5(AS + createEncryptStr(params) + EncodeByteMd5(body) + AS)
	}
	
	func createEncryptStr(params url.Values) string {
		var key []string
		var str = ""
		for k := range params {
			if k != "sn" && k != "debug" {
				key = append(key, k)
			}
		}
		sort.Strings(key)
		for i := 0; i < len(key); i++ {
			if i == 0 {
				str = fmt.Sprintf("%v=%v", key[i], params.Get(key[i]))
			} else {
				str = str + fmt.Sprintf("&%v=%v", key[i], params.Get(key[i]))
			}
		}
		return str
	}
	
	// 由于使用bee2生成，简单加密避免引入过多包，直接在这定义
	// Encode string to md5 hex value
	func EncodeStrMd5(str string) string {
		m := md5.New()
		m.Write([]byte(str))
		return hex.EncodeToString(m.Sum(nil))
	}
	
	func EncodeByteMd5(b []byte) string {
		m := md5.New()
		m.Write(b)
		return hex.EncodeToString(m.Sum(nil))
	}
	
`
)
