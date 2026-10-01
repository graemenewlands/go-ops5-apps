package data

// ColumnDef represents a column/field in a relational table schema.
type ColumnDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	IsPK        bool   `json:"isPk"`
	IsFK        bool   `json:"isFk"`
	FKTable     string `json:"fkTable,omitempty"`
	FKColumn    string `json:"fkColumn,omitempty"`
	Description string `json:"description,omitempty"`
}

// TableDef represents an entity table in the relational schema.
type TableDef struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"displayName"`
	PKColumn    string      `json:"pkColumn"`
	Description string      `json:"description,omitempty"`
	X           float64     `json:"x"`
	Y           float64     `json:"y"`
	Columns     []ColumnDef `json:"columns"`
}

// RelationDef represents a foreign key relationship between two tables.
type RelationDef struct {
	Name        string `json:"name"`
	FromTable   string `json:"fromTable"`
	FromColumn  string `json:"fromColumn"`
	ToTable     string `json:"toTable"`
	ToColumn    string `json:"toColumn"`
	Cardinality string `json:"cardinality"` // "1-N", "N-1", "1-1"
}

// MNRelationDef represents a Many-to-Many junction relationship.
type MNRelationDef struct {
	JunctionTable string `json:"junctionTable"`
	LeftTable     string `json:"leftTable"`
	LeftFK        string `json:"leftFk"`
	RightTable    string `json:"rightTable"`
	RightFK       string `json:"rightFk"`
}

// PresetSelection represents a preconfigured query selection for one-click demo.
type PresetSelection struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Fields      [][2]string `json:"fields"` // [ [table, column], ... ]
}

// SchemaDef represents a complete relational schema with tables, relationships, and sample data.
type SchemaDef struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	Tables      []TableDef                  `json:"tables"`
	Relations   []RelationDef               `json:"relations"`
	MNRelations []MNRelationDef             `json:"mnRelations"`
	SampleData  map[string][]map[string]any `json:"sampleData"`
	Presets     []PresetSelection           `json:"presets"`
}

// GetSchema returns a SchemaDef by ID ("chinook" or "northwind").
func GetSchema(id string) *SchemaDef {
	switch id {
	case "northwind":
		return GetNorthwindSchema()
	case "chinook":
		fallthrough
	default:
		return GetChinookSchema()
	}
}

// AvailableSchemas returns summaries of available schemas.
func AvailableSchemas() []map[string]string {
	return []map[string]string{
		{
			"id":          "chinook",
			"name":        "Chinook (Digital Media Store)",
			"description": "11 tables representing artists, albums, tracks, invoices, customers, and playlist many-to-many associations.",
		},
		{
			"id":          "northwind",
			"name":        "Northwind (E-Commerce & Orders)",
			"description": "Classic relational schema with orders, order details, customers, products, categories, suppliers, and employees.",
		},
	}
}
