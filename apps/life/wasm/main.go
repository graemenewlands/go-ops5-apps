//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/graemenewlands/go-ops5-apps/apps/life"
)

var currentEngine *life.LifeEngine

func main() {
	c := make(chan struct{}, 0)

	// Register JS callbacks
	js.Global().Set("lifeInit", js.FuncOf(lifeInit))
	js.Global().Set("lifeStep", js.FuncOf(lifeStep))
	js.Global().Set("lifeToggleCell", js.FuncOf(lifeToggleCell))
	js.Global().Set("lifeSetCell", js.FuncOf(lifeSetCell))
	js.Global().Set("lifeClear", js.FuncOf(lifeClear))
	js.Global().Set("lifeLoadPattern", js.FuncOf(lifeLoadPattern))
	js.Global().Set("lifeGetState", js.FuncOf(lifeGetState))
	js.Global().Set("lifeGetRuleSource", js.FuncOf(lifeGetRuleSource))

	// Signal to JS that Wasm module is loaded and ready
	if readyFn := js.Global().Get("onOps5LifeReady"); readyFn.Type() == js.TypeFunction {
		readyFn.Invoke()
	}

	<-c
}

func lifeInit(this js.Value, args []js.Value) any {
	width := 30
	height := 30
	rules := life.LifeRulesSource
	if len(args) >= 2 {
		width = args[0].Int()
		height = args[1].Int()
	}
	if len(args) >= 3 && args[2].Type() == js.TypeString && args[2].String() != "" {
		rules = args[2].String()
	}

	eng, err := life.NewLifeEngineWithRules(width, height, rules)
	if err != nil {
		return map[string]any{
			"success": false,
			"error":   err.Error(),
		}
	}
	currentEngine = eng

	return map[string]any{
		"success": true,
		"width":   width,
		"height":  height,
	}
}

func lifeStep(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}

	stats, err := currentEngine.Step()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	return formatStats(stats)
}

func lifeToggleCell(this js.Value, args []js.Value) any {
	if currentEngine == nil || len(args) < 2 {
		return false
	}
	r := args[0].Int()
	c := args[1].Int()
	return currentEngine.ToggleCell(r, c)
}

func lifeSetCell(this js.Value, args []js.Value) any {
	if currentEngine == nil || len(args) < 3 {
		return false
	}
	r := args[0].Int()
	c := args[1].Int()
	alive := args[2].Bool()
	currentEngine.SetCell(r, c, alive)
	return true
}

func lifeClear(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return false
	}
	currentEngine.Clear()
	return true
}

func lifeLoadPattern(this js.Value, args []js.Value) any {
	if currentEngine == nil || len(args) < 1 {
		return map[string]any{"success": false, "error": "missing arguments"}
	}
	name := args[0].String()
	startR := 0
	startC := 0
	if len(args) >= 3 {
		startR = args[1].Int()
		startC = args[2].Int()
	}

	err := currentEngine.LoadPattern(name, startR, startC)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}
	}

	return formatStats(currentEngine.Stats())
}

func lifeGetState(this js.Value, args []js.Value) any {
	if currentEngine == nil {
		return map[string]any{"success": false, "error": "engine not initialized"}
	}
	return formatStats(currentEngine.Stats())
}

func lifeGetRuleSource(this js.Value, args []js.Value) any {
	return life.LifeRulesSource
}

func formatStats(s life.StepStats) map[string]any {
	cellsJS := make([]any, len(s.Cells))
	for i, coord := range s.Cells {
		cellsJS[i] = []any{coord[0], coord[1]}
	}

	return map[string]any{
		"success":     true,
		"generation":  s.Generation,
		"liveCount":   s.LiveCount,
		"stepCycles":  s.StepCycles,
		"totalCycles": s.TotalCycles,
		"stepTimeMs":  fmt.Sprintf("%.2f", s.StepTimeMs),
		"wmeCount":    s.WMECount,
		"cells":       cellsJS,
	}
}
