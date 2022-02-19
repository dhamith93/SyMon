package memdb_test

import (
	"testing"

	"github.com/dhamith93/SyMon/pkg/memdb"
)

func createDatabase() memdb.Database {
	db := memdb.CreateDatabase("testdb")
	err := db.Create(
		"test_table",
		memdb.Col{Name: "int_col", Type: memdb.Int},
		memdb.Col{Name: "float_col", Type: memdb.Float64},
		memdb.Col{Name: "int64_col", Type: memdb.Int},
		memdb.Col{Name: "bool_col", Type: memdb.Bool},
		memdb.Col{Name: "str_col", Type: memdb.String},
	)
	if err != nil {
		return memdb.Database{}
	}
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 1, "test", 45.5)
	db.Tables["test_table"].Insert("bool_col,str_col, float_col", true, "test", 45.5)
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 3, "test", 45.55555625)
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 4, "test1", 41.5)
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 5, "test", 42.5)
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 6, "test", 50.5)
	db.Tables["test_table"].Insert("int_col, str_col, float_col", 7, "test", 50.5)
	return db
}

func TestCreateDatabase(t *testing.T) {
	got := memdb.CreateDatabase("test").Name
	want := "test"
	if got != want {
		t.Errorf("Table name was incorrect, got: %s, want: %s.", got, want)
	}
}

func TestCreate(t *testing.T) {
	db := memdb.CreateDatabase("testdb")
	err := db.Create("test_table", memdb.Col{Name: "int_col", Type: memdb.Int}, memdb.Col{Name: "str_col", Type: memdb.String})

	if err != nil {
		t.Errorf("Failed to create the table. Error: %v", err)
	}

	table, ok := db.Tables["test_table"]

	if !ok {
		t.Errorf("Failed to create the table. Table: %v", db.Tables)
	}

	if len(table.Columns) != 2 {
		t.Errorf("Failed to create columns, got: %d, want: %d", len(table.Columns), 2)
	}

	if table.Columns["int_col"].Type != memdb.Int {
		t.Errorf("Invalid column type, got: %s, want: %s", table.Columns["int_col"].Type, memdb.Int)
	}
}

func TestInsert(t *testing.T) {
	db := memdb.CreateDatabase("testdb")
	err := db.Create("test_table", memdb.Col{Name: "int_col", Type: memdb.Int}, memdb.Col{Name: "str_col", Type: memdb.String})

	if err != nil {
		t.Errorf("Failed to create the table. Error: %v", err)
	}

	err = db.Tables["test_table"].Insert("int_col, str_col", 1, "test")

	if err != nil {
		t.Errorf("Failed to insert to the table. Error: %v", err)
	}
}

func TestInsert2(t *testing.T) {
	db := memdb.CreateDatabase("testdb")
	err := db.Create("test_table", memdb.Col{Name: "int_col", Type: memdb.Int}, memdb.Col{Name: "str_col", Type: memdb.String})

	if err != nil {
		t.Errorf("Failed to create the table. Error: %v", err)
	}

	err = db.Tables["test_table"].Insert("int_col, str_col", "1", "test")

	if err == nil {
		t.Errorf("Failed to type check when inserting values to the table.")
	}
}

func TestSelectAll(t *testing.T) {
	db := memdb.CreateDatabase("testdb")
	err := db.Create("test_table", memdb.Col{Name: "int_col", Type: memdb.Int}, memdb.Col{Name: "str_col", Type: memdb.String})

	if err != nil {
		t.Errorf("Failed to create the table. Error: %v", err)
	}

	db.Tables["test_table"].Insert("int_col, str_col", 1, "test")

	res := db.Tables["test_table"].Select("*")

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 1 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
	}
}

func TestSelectColumns(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Select("int_col")

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 6 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
	}

	expected := [6]int{1, 3, 4, 5, 6, 7}

	for i, r := range res.Rows {
		for _, c := range r.Columns {
			if c.Name != "int_col" {
				t.Errorf("Got invalid column. Res: %v", res.Rows)
			}

			if c.IntVal != expected[i] {
				t.Errorf("Got invalid value. got: %d, want: %d", c.IntVal, expected[i])
			}
		}
	}
}

func TestWhereAll(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("int_col", "==", 4)

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 1 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["int_col"].IntVal != 4 {
		t.Errorf("Got invalid value. got: %d, want: %d", res.Rows[0].Columns["int_col"].IntVal, 4)
	}
}

func TestWhereOne(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("int_col", "==", 4).Select("float_col")

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 1 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["float_col"].Float64Val != 41.5 {
		t.Errorf("Got invalid value. got: %f, want: %f", res.Rows[0].Columns["float_col"].Float64Val, 41.5)
		return
	}
}

func TestWhereOneMultiSelect(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("int_col", "==", 4).Select("float_col, str_col")

	if res.RowCount != 1 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["float_col"].Float64Val != 41.5 {
		t.Errorf("Got invalid value. got: %f, want: %f", res.Rows[0].Columns["float_col"].Float64Val, 41.5)
		return
	}

	if res.Rows[0].Columns["str_col"].StringVal != "test1" {
		t.Errorf("Got invalid value. got: %s, want: %s", res.Rows[0].Columns["str_col"].StringVal, "test1")
		return
	}
}

func TestWhereMultiResultMultiSelect(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("int_col", ">=", 3).Select("float_col, str_col")

	if res.RowCount != 5 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["float_col"].Float64Val != 45.55555625 {
		t.Errorf("Got invalid value. got: %f, want: %f", res.Rows[0].Columns["float_col"].Float64Val, 45.55555625)
		return
	}

	if res.Rows[1].Columns["str_col"].StringVal != "test1" {
		t.Errorf("Got invalid value. got: %s, want: %s", res.Rows[1].Columns["str_col"].StringVal, "test1")
	}
}

func TestWhereAnd(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("str_col", "==", "test").And("float_col", "==", 50.5).Select("float_col")

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 2 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["float_col"].Float64Val != 50.5 && res.Rows[1].Columns["float_col"].Float64Val != 50.5 {
		t.Errorf("Got invalid value. got: %f, want: %f", res.Rows[0].Columns["float_col"].Float64Val, 50.5)
		return
	}
}

func TestWhereOr(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("str_col", "==", "test").And("float_col", "==", 50.5).Or("float_col", "==", 42.5).Select("float_col")

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	if res.RowCount != 3 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	if res.Rows[0].Columns["float_col"].Float64Val != 50.5 && res.Rows[2].Columns["float_col"].Float64Val != 42.5 {
		t.Errorf("Got invalid value. got: %f, want: %f", res.Rows[0].Columns["float_col"].Float64Val, 50.5)
		return
	}
}

func TestUpdate(t *testing.T) {
	db := createDatabase()
	res := db.Tables["test_table"].Where("int_col", ">=", 4)

	if res.Error != nil {
		t.Errorf("Failed to read all values from the table: %v", res.Error)
		return
	}

	res.Update("int_col", 10)

	if res.Error != nil {
		t.Errorf("Failed to update values of the table: %v", res.Error)
		return
	}

	res = db.Tables["test_table"].Where("int_col", ">=", 4).Select("int_col")

	if res.RowCount != 4 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	for _, row := range res.Rows {
		if row.Columns["int_col"].IntVal != 10 {
			t.Errorf("Got invalid value. got: %d, want: %d", row.Columns["int_col"].IntVal, 10)
		}
	}

	res = db.Tables["test_table"].Where("int_col", "<", 4).Select("int_col")

	if res.RowCount != 2 {
		t.Errorf("Failed to read all values from the table. Res: %v", res.Rows)
		return
	}

	for _, row := range res.Rows {
		if row.Columns["int_col"].IntVal == 10 {
			t.Errorf("Got invalid value. got: %d, want: %d", 10, row.Columns["int_col"].IntVal)
		}
	}
}

func TestDeleteNone(t *testing.T) {
	db := createDatabase()
	db.Tables["test_table"].Where("int_col", "==", 2).Delete()

	if len(db.Tables["test_table"].Rows) != 7 {
		t.Errorf("Failed to delete from table. Res: %v", db.Tables["test_table"].Rows)
	}
}

func TestDeleteOne(t *testing.T) {
	db := createDatabase()
	db.Tables["test_table"].Where("int_col", "==", 3).Delete()

	if len(db.Tables["test_table"].Rows) != 6 {
		t.Errorf("Failed to delete from table. Res: %v", db.Tables["test_table"].Rows)
	}
}

func TestDeleteAll(t *testing.T) {
	db := createDatabase()
	db.Tables["test_table"].Where("float_col", ">", 1.0).Delete()

	if len(db.Tables["test_table"].Rows) != 0 {
		t.Errorf("Failed to delete from table. Res: %v", db.Tables["test_table"].Rows)
	}
}

func BenchmarkInsert(b *testing.B) {
	db := createDatabase()
	for i := 0; i < b.N; i++ {
		db.Tables["test_table"].Insert("int_col, str_col, float_col", i, "test", float64(i)+0.5)
	}
}

func BenchmarkDelete(b *testing.B) {
	db := memdb.CreateDatabase("testdb")
	db.Create(
		"test_table",
		memdb.Col{Name: "int_col", Type: memdb.Int},
		memdb.Col{Name: "float_col", Type: memdb.Float64},
		memdb.Col{Name: "int64_col", Type: memdb.Int},
		memdb.Col{Name: "bool_col", Type: memdb.Bool},
		memdb.Col{Name: "str_col", Type: memdb.String},
	)
	for i := 0; i < b.N; i++ {
		db.Tables["test_table"].Insert("int_col, str_col, float_col", 1, "test", float64(i)+0.5)
		db.Tables["test_table"].Where("int_col", "==", 1).Delete()
	}

}

func BenchmarkUpdate(b *testing.B) {
	db := createDatabase()
	for i := 0; i < b.N; i++ {
		db.Tables["test_table"].Insert("int_col, str_col, float_col", i, "test", float64(i)+0.5)
	}

	db.Tables["test_table"].Where("int_col", ">=", 10).Update("int_col", 5000)
}
