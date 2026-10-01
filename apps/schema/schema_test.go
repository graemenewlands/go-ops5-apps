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
