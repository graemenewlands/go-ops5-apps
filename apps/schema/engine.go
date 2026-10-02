package schema

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/graemenewlands/go-ops5-apps/apps/schema/data"
	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

//go:embed rules/schema.ops
var SchemaRulesSource string

// MaterializedColumn describes a column in the generated materialized view.
type MaterializedColumn struct {
	Name        string `json:"name"`
	SourceTable string `json:"sourceTable"`
	SourceCol   string `json:"sourceCol"`
	Type        string `json:"type"`
	IsPK        bool   `json:"isPk"`
}

// MaterializedJoin describes a join condition inferred by OPS5 rules.
type MaterializedJoin struct {
	LeftTable  string `json:"leftTable"`
	LeftCol    string `json:"leftCol"`
	RightTable string `json:"rightTable"`
	RightCol   string `json:"rightCol"`
	JoinType   string `json:"joinType"` // "INNER JOIN", "LEFT JOIN"
}

// ViewStats records execution metrics from the OPS5 rule engine.
type ViewStats struct {
	CycleCount     int     `json:"cycleCount"`
	ElapsedMs      float64 `json:"elapsedMs"`
	WMECount       int     `json:"wmeCount"`
	InferredTables int     `json:"inferredTables"`
	InferredJoins  int     `json:"inferredJoins"`
	ProjectedCols  int     `json:"projectedCols"`
}

// MaterializedViewResult contains the complete synthesized view specification and sample rows.
type MaterializedViewResult struct {
	Success        bool                 `json:"success"`
	Error          string               `json:"error,omitempty"`
	SchemaID       string               `json:"schemaId"`
	SchemaName     string               `json:"schemaName"`
	ViewName       string               `json:"viewName"`
	RootTable      string               `json:"rootTable"`
	Columns        []MaterializedColumn `json:"columns"`
	Joins          []MaterializedJoin   `json:"joins"`
	SQL            string               `json:"sql"`
	SampleRows     []map[string]any     `json:"sampleRows"`
	Stats          ViewStats            `json:"stats"`
	NeededTables   []string             `json:"neededTables"`
	SelectedFields [][2]string          `json:"selectedFields"`
}

// SchemaEngine manages an OPS5 engine instance loaded with database schema WMEs and rules.
type SchemaEngine struct {
	mu             sync.RWMutex
	schemaDef      *data.SchemaDef
	eng            *engine.Engine
	selectedFields [][2]string // [ [Table, Column], ... ]
}

// NewSchemaEngine creates an initialized engine for the requested schema.
func NewSchemaEngine(schemaID string) (*SchemaEngine, error) {
	schemaDef := data.GetSchema(schemaID)
	if schemaDef == nil {
		return nil, fmt.Errorf("unknown schema ID: %s", schemaID)
	}

	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	_ = eng.SetWatchLevel(0)

	rules, err := parser.ParseRules(SchemaRulesSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema rules: %w", err)
	}
	for _, r := range rules {
		eng.AddRule(r)
	}

	se := &SchemaEngine{
		schemaDef: schemaDef,
		eng:       eng,
	}

	se.assertBaseSchema()
	return se, nil
}

// SchemaDef returns the underlying schema metadata.
func (se *SchemaEngine) SchemaDef() *data.SchemaDef {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.schemaDef
}

// SwitchSchema switches to another schema ("chinook" or "northwind") and reloads WMEs.
func (se *SchemaEngine) SwitchSchema(schemaID string) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	newDef := data.GetSchema(schemaID)
	if newDef == nil {
		return fmt.Errorf("unknown schema ID: %s", schemaID)
	}

	se.schemaDef = newDef
	se.selectedFields = nil

	// Re-instantiate engine
	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	_ = eng.SetWatchLevel(0)

	rules, err := parser.ParseRules(SchemaRulesSource)
	if err != nil {
		return fmt.Errorf("failed to parse schema rules: %w", err)
	}
	for _, r := range rules {
		eng.AddRule(r)
	}
	se.eng = eng
	se.assertBaseSchema()
	return nil
}

// assertBaseSchema asserts tables, fields, relations, and M-N relationships into working memory.
func (se *SchemaEngine) assertBaseSchema() {
	// 1. Assert tables
	for _, t := range se.schemaDef.Tables {
		se.eng.Make("table", map[string]model.Value{
			"name":    model.NewSymbol(t.Name),
			"display": model.NewSymbol(t.DisplayName),
			"pk":      model.NewSymbol(t.PKColumn),
		})

		// Assert fields
		for _, f := range t.Columns {
			isPKVal := int64(0)
			if f.IsPK {
				isPKVal = 1
			}
			isFKVal := int64(0)
			if f.IsFK {
				isFKVal = 1
			}
			se.eng.Make("field", map[string]model.Value{
				"table": model.NewSymbol(t.Name),
				"name":  model.NewSymbol(f.Name),
				"type":  model.NewSymbol(f.Type),
				"is_pk": model.NewInt(isPKVal),
				"is_fk": model.NewInt(isFKVal),
			})
		}
	}

	// 2. Assert relations
	for _, r := range se.schemaDef.Relations {
		se.eng.Make("relation", map[string]model.Value{
			"name":       model.NewSymbol(r.Name),
			"from_table": model.NewSymbol(r.FromTable),
			"from_col":   model.NewSymbol(r.FromColumn),
			"to_table":   model.NewSymbol(r.ToTable),
			"to_col":     model.NewSymbol(r.ToColumn),
			"type":       model.NewSymbol(r.Cardinality),
		})
	}

	// 3. Assert M-N relationships
	for _, mn := range se.schemaDef.MNRelations {
		se.eng.Make("mn_relationship", map[string]model.Value{
			"junction_table": model.NewSymbol(mn.JunctionTable),
			"left_table":     model.NewSymbol(mn.LeftTable),
			"left_fk":        model.NewSymbol(mn.LeftFK),
			"right_table":    model.NewSymbol(mn.RightTable),
			"right_fk":       model.NewSymbol(mn.RightFK),
		})
	}
}

// SetSelectedFields updates the target fields selected by the user.
func (se *SchemaEngine) SetSelectedFields(fields [][2]string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.selectedFields = fields
}

// GetSelectedFields returns the currently selected fields.
func (se *SchemaEngine) GetSelectedFields() [][2]string {
	se.mu.RLock()
	defer se.mu.RUnlock()
	res := make([][2]string, len(se.selectedFields))
	copy(res, se.selectedFields)
	return res
}

// Clear removes all transient inference WMEs and clears selected fields.
func (se *SchemaEngine) Clear() {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.selectedFields = nil
	se.clearTransientWMEs()
}

// clearTransientWMEs removes all inference and control WMEs from working memory.
func (se *SchemaEngine) clearTransientWMEs() {
	transientClasses := []string{
		"query_control",
		"selected_field",
		"needed_table",
		"active_join",
		"view_column",
		"materialized_view",
	}
	for _, cls := range transientClasses {
		wmes := se.eng.WorkingMemory().FindByClass(cls)
		for _, w := range wmes {
			_, _ = se.eng.Remove(w.Timetag)
		}
	}
}

// GenerateMaterializedView executes OPS5 production rules to infer joins and synthesize the view.
func (se *SchemaEngine) GenerateMaterializedView(selectedFields [][2]string) (*MaterializedViewResult, error) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if len(selectedFields) == 0 {
		return &MaterializedViewResult{
			Success:        false,
			Error:          "No fields selected. Please select at least one field on the E-R diagram.",
			SchemaID:       se.schemaDef.ID,
			SchemaName:     se.schemaDef.Name,
			Columns:        make([]MaterializedColumn, 0),
			Joins:          make([]MaterializedJoin, 0),
			SampleRows:     make([]map[string]any, 0),
			NeededTables:   make([]string, 0),
			SelectedFields: make([][2]string, 0),
		}, nil
	}

	se.selectedFields = selectedFields

	// Clear previous transient inference WMEs
	se.clearTransientWMEs()

	// Look up types for selected fields
	colTypeMap := make(map[string]string)
	for _, t := range se.schemaDef.Tables {
		for _, c := range t.Columns {
			key := fmt.Sprintf("%s.%s", t.Name, c.Name)
			colTypeMap[key] = c.Type
		}
	}

	// Assert selected_field WMEs
	for _, sf := range selectedFields {
		tbl := sf[0]
		col := sf[1]
		typ := colTypeMap[fmt.Sprintf("%s.%s", tbl, col)]
		if typ == "" {
			typ = "VARCHAR"
		}
		se.eng.Make("selected_field", map[string]model.Value{
			"table":  model.NewSymbol(tbl),
			"column": model.NewSymbol(col),
			"type":   model.NewSymbol(typ),
		})
	}

	// Assert control WME to trigger rule execution
	se.eng.Make("query_control", map[string]model.Value{
		"phase":  model.NewSymbol("identify-tables"),
		"status": model.NewSymbol("running"),
	})

	startTime := time.Now()
	initialCycles := se.eng.CycleCount()

	// Run rule engine until quiescence
	_, err := se.eng.Run(0)
	if err != nil {
		return nil, fmt.Errorf("error executing OPS5 schema rules: %w", err)
	}

	elapsed := time.Since(startTime)
	cyclesFired := se.eng.CycleCount() - initialCycles

	// Extract results from working memory
	neededTableWMEs := se.eng.WorkingMemory().FindByClass("needed_table")
	neededTables := make([]string, 0, len(neededTableWMEs))
	for _, w := range neededTableWMEs {
		if nameVal, ok := w.Get("name"); ok {
			neededTables = append(neededTables, fmt.Sprintf("%v", nameVal.Raw()))
		}
	}
	sort.Strings(neededTables)

	// Determine root table
	rootTable := ""
	mvWMEs := se.eng.WorkingMemory().FindByClass("materialized_view")
	if len(mvWMEs) > 0 {
		if rootVal, ok := mvWMEs[0].Get("root_table"); ok {
			rootTable = fmt.Sprintf("%v", rootVal.Raw())
		}
	}
	if rootTable == "" && len(neededTables) > 0 {
		rootTable = neededTables[0]
	}

	// Extract active joins
	joinWMEs := se.eng.WorkingMemory().FindByClass("active_join")
	joins := make([]MaterializedJoin, 0)
	seenJoins := make(map[string]bool)

	for _, w := range joinWMEs {
		ltVal, _ := w.Get("left_table")
		lcVal, _ := w.Get("left_col")
		rtVal, _ := w.Get("right_table")
		rcVal, _ := w.Get("right_col")
		jtVal, _ := w.Get("join_type")

		lt := fmt.Sprintf("%v", ltVal.Raw())
		lc := fmt.Sprintf("%v", lcVal.Raw())
		rt := fmt.Sprintf("%v", rtVal.Raw())
		rc := fmt.Sprintf("%v", rcVal.Raw())
		jt := strings.ToUpper(fmt.Sprintf("%v", jtVal.Raw())) + " JOIN"

		key1 := fmt.Sprintf("%s.%s=%s.%s", lt, lc, rt, rc)
		key2 := fmt.Sprintf("%s.%s=%s.%s", rt, rc, lt, lc)
		if seenJoins[key1] || seenJoins[key2] {
			continue
		}
		seenJoins[key1] = true

		joins = append(joins, MaterializedJoin{
			LeftTable:  lt,
			LeftCol:    lc,
			RightTable: rt,
			RightCol:   rc,
			JoinType:   jt,
		})
	}

	// Order joins sequentially starting from rootTable
	orderedJoins := orderJoinsFromRoot(rootTable, joins)

	// Extract projected columns & compute unique aliases
	colWMEs := se.eng.WorkingMemory().FindByClass("view_column")
	rawCols := make([]MaterializedColumn, 0, len(colWMEs))
	colNameFreq := make(map[string]int)

	for _, w := range colWMEs {
		nameVal, _ := w.Get("name")
		stVal, _ := w.Get("source_table")
		scVal, _ := w.Get("source_col")
		typVal, _ := w.Get("type")

		st := fmt.Sprintf("%v", stVal.Raw())
		sc := fmt.Sprintf("%v", scVal.Raw())
		cName := fmt.Sprintf("%v", nameVal.Raw())
		cType := fmt.Sprintf("%v", typVal.Raw())

		// Check if it's a primary key in its table
		isPK := false
		for _, t := range se.schemaDef.Tables {
			if t.Name == st && t.PKColumn == sc {
				isPK = true
				break
			}
		}

		rawCols = append(rawCols, MaterializedColumn{
			Name:        cName,
			SourceTable: st,
			SourceCol:   sc,
			Type:        cType,
			IsPK:        isPK,
		})
		colNameFreq[cName]++
	}

	// Assign disambiguated column aliases if duplicates exist
	projectedCols := make([]MaterializedColumn, 0, len(rawCols))
	for _, c := range rawCols {
		finalName := c.Name
		if colNameFreq[c.Name] > 1 {
			finalName = fmt.Sprintf("%s_%s", strings.ToLower(c.SourceTable), strings.ToLower(c.SourceCol))
		}
		projectedCols = append(projectedCols, MaterializedColumn{
			Name:        finalName,
			SourceTable: c.SourceTable,
			SourceCol:   c.SourceCol,
			Type:        c.Type,
			IsPK:        c.IsPK,
		})
	}

	// Preserve user selection order
	sort.Slice(projectedCols, func(i, j int) bool {
		idxI := findSelectionIndex(selectedFields, projectedCols[i].SourceTable, projectedCols[i].SourceCol)
		idxJ := findSelectionIndex(selectedFields, projectedCols[j].SourceTable, projectedCols[j].SourceCol)
		return idxI < idxJ
	})

	// Synthesize formatted SQL
	sqlStmt := buildMaterializedViewSQL("mv_denormalized_data", rootTable, orderedJoins, projectedCols)

	// Materialize sample data rows using the inferred joins
	sampleRows := se.materializeSampleRows(rootTable, orderedJoins, projectedCols)
	if sampleRows == nil {
		sampleRows = make([]map[string]any, 0)
	}

	result := &MaterializedViewResult{
		Success:        true,
		SchemaID:       se.schemaDef.ID,
		SchemaName:     se.schemaDef.Name,
		ViewName:       "mv_denormalized_data",
		RootTable:      rootTable,
		Columns:        projectedCols,
		Joins:          orderedJoins,
		SQL:            sqlStmt,
		SampleRows:     sampleRows,
		NeededTables:   neededTables,
		SelectedFields: selectedFields,
		Stats: ViewStats{
			CycleCount:     cyclesFired,
			ElapsedMs:      float64(elapsed.Microseconds()) / 1000.0,
			WMECount:       se.eng.WorkingMemory().Count(),
			InferredTables: len(neededTables),
			InferredJoins:  len(orderedJoins),
			ProjectedCols:  len(projectedCols),
		},
	}

	return result, nil
}

func findSelectionIndex(selected [][2]string, table, col string) int {
	for i, s := range selected {
		if s[0] == table && s[1] == col {
			return i
		}
	}
	return len(selected)
}

// orderJoinsFromRoot sorts and orients joins so each joined table attaches to an already reached table.
func orderJoinsFromRoot(root string, joins []MaterializedJoin) []MaterializedJoin {
	if len(joins) == 0 {
		return make([]MaterializedJoin, 0)
	}

	reached := map[string]bool{root: true}
	ordered := make([]MaterializedJoin, 0, len(joins))
	remaining := make([]MaterializedJoin, len(joins))
	copy(remaining, joins)

	for len(remaining) > 0 {
		found := false
		for i, jn := range remaining {
			if reached[jn.LeftTable] && !reached[jn.RightTable] {
				reached[jn.RightTable] = true
				ordered = append(ordered, jn)
				remaining = append(remaining[:i], remaining[i+1:]...)
				found = true
				break
			} else if reached[jn.RightTable] && !reached[jn.LeftTable] {
				// Invert join order so Left is already in tree, Right is newly added
				reached[jn.LeftTable] = true
				ordered = append(ordered, MaterializedJoin{
					LeftTable:  jn.RightTable,
					LeftCol:    jn.RightCol,
					RightTable: jn.LeftTable,
					RightCol:   jn.LeftCol,
					JoinType:   jn.JoinType,
				})
				remaining = append(remaining[:i], remaining[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			// Disconnected or cyclic, append remaining
			ordered = append(ordered, remaining...)
			break
		}
	}
	return ordered
}

// buildMaterializedViewSQL synthesizes standard PostgreSQL / Oracle / Redshift CREATE MATERIALIZED VIEW statement.
func buildMaterializedViewSQL(viewName, rootTable string, joins []MaterializedJoin, cols []MaterializedColumn) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("-- Auto-synthesized by OPS5 Forward-Chaining Production Rules\n"))
	sb.WriteString(fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS\nSELECT\n", viewName))

	for i, c := range cols {
		comma := ","
		if i == len(cols)-1 {
			comma = ""
		}
		if strings.EqualFold(c.Name, c.SourceCol) {
			sb.WriteString(fmt.Sprintf("    %s.%s%s\n", c.SourceTable, c.SourceCol, comma))
		} else {
			sb.WriteString(fmt.Sprintf("    %s.%s AS %s%s\n", c.SourceTable, c.SourceCol, c.Name, comma))
		}
	}

	sb.WriteString(fmt.Sprintf("FROM %s\n", rootTable))

	for _, j := range joins {
		sb.WriteString(fmt.Sprintf("%s %s\n    ON %s.%s = %s.%s\n",
			j.JoinType, j.RightTable, j.LeftTable, j.LeftCol, j.RightTable, j.RightCol))
	}
	sb.WriteString(";")

	return sb.String()
}

// materializeSampleRows computes in-memory join over the embedded sample dataset.
func (se *SchemaEngine) materializeSampleRows(rootTable string, joins []MaterializedJoin, cols []MaterializedColumn) []map[string]any {
	rootData := se.schemaDef.SampleData[rootTable]
	if len(rootData) == 0 {
		return make([]map[string]any, 0)
	}

	// Working state: slice of composite rows with prefix Table.Column
	type compositeRow map[string]any
	var currentRows []compositeRow

	for _, r := range rootData {
		cr := make(compositeRow)
		for k, v := range r {
			cr[fmt.Sprintf("%s.%s", rootTable, k)] = v
		}
		currentRows = append(currentRows, cr)
	}

	// Iteratively join each table in the join tree
	for _, j := range joins {
		rightData := se.schemaDef.SampleData[j.RightTable]
		var nextRows []compositeRow

		leftKey := fmt.Sprintf("%s.%s", j.LeftTable, j.LeftCol)

		for _, cr := range currentRows {
			leftVal, exists := cr[leftKey]
			if !exists {
				continue
			}

			matched := false
			for _, r := range rightData {
				rVal := r[j.RightCol]
				if compareKeys(leftVal, rVal) {
					matched = true
					newCR := make(compositeRow, len(cr)+len(r))
					for k, v := range cr {
						newCR[k] = v
					}
					for k, v := range r {
						newCR[fmt.Sprintf("%s.%s", j.RightTable, k)] = v
					}
					nextRows = append(nextRows, newCR)
				}
			}

			// If LEFT JOIN and no matches, retain with nulls
			if !matched && strings.HasPrefix(j.JoinType, "LEFT") {
				newCR := make(compositeRow, len(cr))
				for k, v := range cr {
					newCR[k] = v
				}
				nextRows = append(nextRows, newCR)
			}
		}
		currentRows = nextRows
	}

	// Project target view columns
	outputRows := make([]map[string]any, 0, len(currentRows))
	for _, cr := range currentRows {
		row := make(map[string]any)
		for _, c := range cols {
			sourceKey := fmt.Sprintf("%s.%s", c.SourceTable, c.SourceCol)
			row[c.Name] = cr[sourceKey]
		}
		outputRows = append(outputRows, row)
	}

	return outputRows
}

func compareKeys(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
