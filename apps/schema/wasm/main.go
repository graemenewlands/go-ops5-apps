//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/graemenewlands/go-ops5-apps/apps/schema"
	"github.com/graemenewlands/go-ops5-apps/apps/schema/data"
)

var currentEngine *schema.SchemaEngine

func main() {
	c := make(chan struct{}, 0)

	// Register JS callbacks
	js.Global().Set("schemaInit", js.FuncOf(schemaInit))
	js.Global().Set("schemaSwitch", js.FuncOf(schemaSwitch))
	js.Global().Set("schemaGetDefinition", js.FuncOf(schemaGetDefinition))
	js.Global().Set("schemaGetAvailableSchemas", js.FuncOf(schemaGetAvailableSchemas))
	js.Global().Set("schemaGenerateView", js.FuncOf(schemaGenerateView))
	js.Global().Set("schemaClear", js.FuncOf(schemaClear))
	js.Global().Set("schemaGetRuleSource", js.FuncOf(schemaGetRuleSource))

	// Signal to JS that Wasm module is loaded and ready
	if readyFn := js.Global().Get("onOps5SchemaReady"); readyFn.Type() == js.TypeFunction {
		readyFn.Invoke()
	}

	<-c
}

func schemaInit(this js.Value, args []js.Value) any {
	schemaID := "chinook"
	if len(args) >= 1 && args[0].Type() == js.TypeString && args[0].String() != "" {
		schemaID = args[0].String()
	}

	eng, err := schema.NewSchemaEngine(schemaID)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}
	}
	currentEngine = eng

	return map[string]any{
		"success":  true,
		"schemaId": schemaID,
	}
}

func schemaSwitch(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return map[string]any{"success": false, "error": "missing schema ID"}
	}

	schemaID := args[0].String()
	err := currentEngine.SwitchSchema(schemaID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	return map[string]any{
		"success":  true,
		"schemaId": schemaID,
	}
}

func schemaGetDefinition(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}

	def := currentEngine.SchemaDef()
	bytes, err := json.Marshal(def)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	var parsed any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	return map[string]any{
		"success": true,
		"schema":  parsed,
	}
}

func schemaGetAvailableSchemas(this js.Value, args []js.Value) any {
	schemas := data.AvailableSchemas()
	var res []any
	for _, s := range schemas {
		res = append(res, map[string]any{
			"id":          s["id"],
			"name":        s["name"],
			"description": s["description"],
		})
	}
	return res
}

func schemaGenerateView(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}

	var selectedFields [][2]string
	if len(args) >= 1 && args[0].Type() == js.TypeString {
		// Parse JSON array of [table, column] pairs
		var fields [][2]string
		if err := json.Unmarshal([]byte(args[0].String()), &fields); err == nil {
			selectedFields = fields
		}
	} else if len(args) >= 1 && args[0].Type() == js.TypeObject {
		// Read array of arrays from JS directly
		length := args[0].Length()
		for i := 0; i < length; i++ {
			item := args[0].Index(i)
			if item.Length() >= 2 {
				selectedFields = append(selectedFields, [2]string{
					item.Index(0).String(),
					item.Index(1).String(),
				})
			}
		}
	}

	res, err := currentEngine.GenerateMaterializedView(selectedFields)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}
	}

	var parsed any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}
	}

	return parsed
}

func schemaClear(this js.Value, args []js.Value) any {
	if currentEngine != nil {
		currentEngine.Clear()
	}
	return map[string]any{"success": true}
}

func schemaGetRuleSource(this js.Value, args []js.Value) any {
	return schema.SchemaRulesSource
}
