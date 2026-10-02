package schema

import (
	"strings"
	"testing"
)

func TestChinookMultiTableJoin(t *testing.T) {
	eng, err := NewSchemaEngine("chinook")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}

	selected := [][2]string{
		{"Customer", "LastName"},
		{"Invoice", "InvoiceDate"},
		{"Track", "Name"},
		{"Album", "Title"},
		{"Artist", "Name"},
	}

	res, err := eng.GenerateMaterializedView(selected)
	if err != nil {
		t.Fatalf("failed to generate materialized view: %v", err)
	}
	if !res.Success {
		t.Fatalf("generation failed: %s", res.Error)
	}

	// Verify columns projected
	if len(res.Columns) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(res.Columns))
	}

	// Verify name disambiguation: "Name" from Track and Artist should be disambiguated
	colNames := make(map[string]bool)
	for _, c := range res.Columns {
		colNames[c.Name] = true
	}
	if !colNames["artist_name"] || !colNames["track_name"] {
		t.Fatalf("expected disambiguated 'artist_name' and 'track_name', got: %v", colNames)
	}

	// Verify joins inferred
	if len(res.Joins) == 0 {
		t.Fatalf("expected joins to be inferred across Customer, Invoice, Track, Album, Artist")
	}

	// Verify sample rows materialized
	if len(res.SampleRows) == 0 {
		t.Fatalf("expected sample rows to be materialized, got 0")
	}

	// Verify SQL contains CREATE MATERIALIZED VIEW
	if !strings.Contains(res.SQL, "CREATE MATERIALIZED VIEW") {
		t.Fatalf("SQL does not contain CREATE MATERIALIZED VIEW: %s", res.SQL)
	}
	if !strings.Contains(res.SQL, "FROM") || !strings.Contains(res.SQL, "JOIN") {
		t.Fatalf("SQL missing FROM or JOIN clauses: %s", res.SQL)
	}

	// Verify OPS5 cycle count > 0
	if res.Stats.CycleCount <= 0 {
		t.Fatalf("expected OPS5 cycle count > 0, got %d", res.Stats.CycleCount)
	}
}

func TestChinookMNPlaylistTracks(t *testing.T) {
	eng, err := NewSchemaEngine("chinook")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}

	selected := [][2]string{
		{"Playlist", "Name"},
		{"Track", "Name"},
	}

	res, err := eng.GenerateMaterializedView(selected)
	if err != nil {
		t.Fatalf("failed to generate materialized view: %v", err)
	}
	if !res.Success {
		t.Fatalf("generation failed: %s", res.Error)
	}

	// Junction table PlaylistTrack should be inferred
	foundJunction := false
	for _, tbl := range res.NeededTables {
		if tbl == "PlaylistTrack" {
			foundJunction = true
			break
		}
	}
	if !foundJunction {
		t.Fatalf("expected PlaylistTrack junction table to be inferred, got: %v", res.NeededTables)
	}

	// Sample rows should match playlist tracks
	if len(res.SampleRows) == 0 {
		t.Fatalf("expected materialized playlist track rows, got 0")
	}
}

func TestNorthwindOrderDetails(t *testing.T) {
	eng, err := NewSchemaEngine("northwind")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}

	selected := [][2]string{
		{"Customers", "CompanyName"},
		{"Orders", "OrderDate"},
		{"Products", "ProductName"},
		{"OrderDetails", "Quantity"},
	}

	res, err := eng.GenerateMaterializedView(selected)
	if err != nil {
		t.Fatalf("failed to generate materialized view: %v", err)
	}
	if !res.Success {
		t.Fatalf("generation failed: %s", res.Error)
	}

	if len(res.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(res.Columns))
	}
	if len(res.Joins) == 0 {
		t.Fatalf("expected joins to be inferred")
	}
	if len(res.SampleRows) == 0 {
		t.Fatalf("expected materialized rows, got 0")
	}
}

func TestSingleTableSelection(t *testing.T) {
	eng, err := NewSchemaEngine("chinook")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}

	selected := [][2]string{
		{"Track", "TrackId"},
		{"Track", "Name"},
		{"Track", "UnitPrice"},
	}

	res, err := eng.GenerateMaterializedView(selected)
	if err != nil {
		t.Fatalf("failed to generate view: %v", err)
	}
	if !res.Success {
		t.Fatalf("generation failed: %s", res.Error)
	}

	if len(res.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(res.Columns))
	}
	if len(res.Joins) != 0 {
		t.Fatalf("expected 0 joins for single table, got %d", len(res.Joins))
	}
	if len(res.SampleRows) == 0 {
		t.Fatalf("expected sample rows, got 0")
	}
}

func TestSequentialQueriesAndSingleField(t *testing.T) {
	eng, err := NewSchemaEngine("chinook")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}

	// 1. Run multi-table query (Customer Music Purchases preset)
	multiFields := [][2]string{
		{"Customer", "LastName"},
		{"Customer", "City"},
		{"Invoice", "InvoiceDate"},
		{"Track", "Name"},
		{"Album", "Title"},
		{"Artist", "Name"},
		{"InvoiceLine", "UnitPrice"},
		{"InvoiceLine", "Quantity"},
	}
	res1, err := eng.GenerateMaterializedView(multiFields)
	if err != nil || !res1.Success {
		t.Fatalf("res1 failed: %v, %v", err, res1)
	}
	if len(res1.Columns) != 8 {
		t.Fatalf("expected 8 columns in res1, got %d", len(res1.Columns))
	}
	if len(res1.Joins) == 0 {
		t.Fatalf("expected joins in res1, got 0")
	}

	// 2. Clear engine state
	eng.Clear()

	// 3. Now run a single field query: CustomerId from Customer
	singleCustomer := [][2]string{
		{"Customer", "CustomerId"},
	}
	res2, err := eng.GenerateMaterializedView(singleCustomer)
	if err != nil || !res2.Success {
		t.Fatalf("res2 failed: %v, %v", err, res2)
	}

	if len(res2.Columns) != 1 {
		t.Fatalf("expected 1 column in res2, got %d: %v", len(res2.Columns), res2.Columns)
	}
	if len(res2.Joins) != 0 {
		t.Fatalf("expected 0 joins in res2, got %d: %v", len(res2.Joins), res2.Joins)
	}
	if res2.RootTable != "Customer" {
		t.Fatalf("expected root table Customer, got %s", res2.RootTable)
	}
	if !strings.Contains(res2.SQL, "FROM Customer") {
		t.Fatalf("expected SQL FROM Customer, got: %s", res2.SQL)
	}
	if !strings.Contains(res2.SQL, "Customer.CustomerId") {
		t.Fatalf("expected SQL to contain Customer.CustomerId, got: %s", res2.SQL)
	}
	if strings.Contains(res2.SQL, "JOIN") {
		t.Fatalf("single table query should not have JOIN in SQL: %s", res2.SQL)
	}

	// 4. Test Employee single field (ensure no self-join on ReportsTo)
	singleEmployee := [][2]string{
		{"Employee", "FirstName"},
	}
	res3, err := eng.GenerateMaterializedView(singleEmployee)
	if err != nil || !res3.Success {
		t.Fatalf("res3 failed: %v, %v", err, res3)
	}
	if len(res3.Columns) != 1 {
		t.Fatalf("expected 1 column in res3, got %d", len(res3.Columns))
	}
	if len(res3.Joins) != 0 {
		t.Fatalf("expected 0 joins in res3 (no self join on ReportsTo), got %d: %v", len(res3.Joins), res3.Joins)
	}
	if res3.RootTable != "Employee" {
		t.Fatalf("expected root table Employee, got %s", res3.RootTable)
	}

	// 5. Test another multi-table query after single fields
	playlistFields := [][2]string{
		{"Playlist", "Name"},
		{"Track", "Name"},
	}
	res4, err := eng.GenerateMaterializedView(playlistFields)
	if err != nil || !res4.Success {
		t.Fatalf("res4 failed: %v, %v", err, res4)
	}
	if len(res4.Columns) != 2 {
		t.Fatalf("expected 2 columns in res4, got %d", len(res4.Columns))
	}
	if len(res4.Joins) < 2 {
		t.Fatalf("expected at least 2 joins for playlist-track M-N, got %d", len(res4.Joins))
	}
}

func TestSchemaSwitch(t *testing.T) {
	eng, err := NewSchemaEngine("chinook")
	if err != nil {
		t.Fatalf("failed to create schema engine: %v", err)
	}
	if eng.SchemaDef().ID != "chinook" {
		t.Fatalf("expected chinook, got %s", eng.SchemaDef().ID)
	}

	err = eng.SwitchSchema("northwind")
	if err != nil {
		t.Fatalf("failed to switch to northwind: %v", err)
	}
	if eng.SchemaDef().ID != "northwind" {
		t.Fatalf("expected northwind, got %s", eng.SchemaDef().ID)
	}

	err = eng.SwitchSchema("chinook")
	if err != nil {
		t.Fatalf("failed to switch back to chinook: %v", err)
	}
	if eng.SchemaDef().ID != "chinook" {
		t.Fatalf("expected chinook, got %s", eng.SchemaDef().ID)
	}
}
