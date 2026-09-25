package life

import (
	_ "embed"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/graemenewlands/ops5/pkg/conflict"
	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
)

//go:embed rules/life.ops
var LifeRulesSource string

// StepStats records performance and state metrics after each generation step.
type StepStats struct {
	Generation  int       `json:"generation"`
	LiveCount   int       `json:"liveCount"`
	StepCycles  int       `json:"stepCycles"`
	TotalCycles int       `json:"totalCycles"`
	StepTimeMs  float64   `json:"stepTimeMs"`
	WMECount    int       `json:"wmeCount"`
	Cells       [][2]int  `json:"cells"`
}

// LifeEngine encapsulates an OPS5 engine running Conway's Game of Life.
type LifeEngine struct {
	mu          sync.RWMutex
	width       int
	height      int
	eng         *engine.Engine
	generation  int
	totalCycles int
	lastStepMs  float64
	stepTimetag int64
	gridTimetag int64
}

// NewLifeEngine creates and initializes a Game of Life simulation with standard rules.
func NewLifeEngine(width, height int) (*LifeEngine, error) {
	return NewLifeEngineWithRules(width, height, LifeRulesSource)
}

// NewLifeEngineWithRules creates a Game of Life simulation with custom OPS5 rules.
func NewLifeEngineWithRules(width, height int, rulesSource string) (*LifeEngine, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid grid dimensions: %dx%d", width, height)
	}

	eng := engine.New()
	eng.SetStrategy(conflict.StrategyLEX)
	_ = eng.SetWatchLevel(0) // Silent execution by default for high-speed animation

	rules, err := parser.ParseRules(rulesSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse life rules: %w", err)
	}
	for _, r := range rules {
		eng.AddRule(r)
	}

	// 1. Assert grid bounds
	gridWME := eng.Make("grid", map[string]model.Value{
		"width":  model.NewInt(int64(width)),
		"height": model.NewInt(int64(height)),
	})

	// 2. Assert 8 Moore neighborhood direction vectors
	deltas := [][2]int64{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}
	for _, d := range deltas {
		eng.Make("delta", map[string]model.Value{
			"dr": model.NewInt(d[0]),
			"dc": model.NewInt(d[1]),
		})
	}

	// 3. Assert initial step control in ready phase
	stepWME := eng.Make("step", map[string]model.Value{
		"phase":      model.NewSymbol("ready"),
		"generation": model.NewInt(0),
	})

	return &LifeEngine{
		width:       width,
		height:      height,
		eng:         eng,
		generation:  0,
		totalCycles: 0,
		lastStepMs:  0,
		stepTimetag: stepWME.Timetag,
		gridTimetag: gridWME.Timetag,
	}, nil
}

// Width returns the grid width.
func (le *LifeEngine) Width() int {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.width
}

// Height returns the grid height.
func (le *LifeEngine) Height() int {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.height
}

// SetCell sets a cell's state at (r, c). Coordinates wrap toroidally.
func (le *LifeEngine) SetCell(r, c int, alive bool) {
	le.mu.Lock()
	defer le.mu.Unlock()

	r = (r%le.height + le.height) % le.height
	c = (c%le.width + le.width) % le.width

	// Check if cell already exists
	cells := le.eng.WorkingMemory().FindByClass("cell")
	for _, w := range cells {
		rowVal, rOk := w.Get("r")
		colVal, cOk := w.Get("c")
		if rOk && cOk && rowVal.Raw() == int64(r) && colVal.Raw() == int64(c) {
			if !alive {
				_, _ = le.eng.Remove(w.Timetag)
			}
			return
		}
	}

	if alive {
		le.eng.Make("cell", map[string]model.Value{
			"r":     model.NewInt(int64(r)),
			"c":     model.NewInt(int64(c)),
			"alive": model.NewInt(1),
		})
	}
}

// ToggleCell toggles the cell at (r, c) between alive and dead.
func (le *LifeEngine) ToggleCell(r, c int) bool {
	le.mu.Lock()
	defer le.mu.Unlock()

	r = (r%le.height + le.height) % le.height
	c = (c%le.width + le.width) % le.width

	cells := le.eng.WorkingMemory().FindByClass("cell")
	for _, w := range cells {
		rowVal, rOk := w.Get("r")
		colVal, cOk := w.Get("c")
		if rOk && cOk && rowVal.Raw() == int64(r) && colVal.Raw() == int64(c) {
			_, _ = le.eng.Remove(w.Timetag)
			return false
		}
	}

	le.eng.Make("cell", map[string]model.Value{
		"r":     model.NewInt(int64(r)),
		"c":     model.NewInt(int64(c)),
		"alive": model.NewInt(1),
	})
	return true
}

// Clear removes all live cells and resets generation counter to 0.
func (le *LifeEngine) Clear() {
	le.mu.Lock()
	defer le.mu.Unlock()

	cells := le.eng.WorkingMemory().FindByClass("cell")
	for _, w := range cells {
		_, _ = le.eng.Remove(w.Timetag)
	}

	le.generation = 0
	steps := le.eng.WorkingMemory().FindByClass("step")
	if len(steps) > 0 {
		modWME, err := le.eng.Modify(steps[0].Timetag, map[string]model.Value{
			"phase":      model.NewSymbol("ready"),
			"generation": model.NewInt(0),
		})
		if err == nil && modWME != nil {
			le.stepTimetag = modWME.Timetag
		}
	}
}

// GetLiveCells returns coordinate pairs [r, c] of all active cells.
func (le *LifeEngine) GetLiveCells() [][2]int {
	le.mu.RLock()
	defer le.mu.RUnlock()

	return le.getLiveCellsLocked()
}

func (le *LifeEngine) getLiveCellsLocked() [][2]int {
	cells := le.eng.WorkingMemory().FindByClass("cell")
	var result [][2]int
	for _, w := range cells {
		aliveVal, aOk := w.Get("alive")
		if aOk && aliveVal.Raw() == int64(1) {
			rVal, _ := w.Get("r")
			cVal, _ := w.Get("c")
			result = append(result, [2]int{int(rVal.Raw().(int64)), int(cVal.Raw().(int64))})
		}
	}
	return result
}

// Step advances the simulation by one generation cycle using OPS5 rules.
func (le *LifeEngine) Step() (StepStats, error) {
	le.mu.Lock()
	defer le.mu.Unlock()

	// 1. Look up active step WME and trigger emit phase
	steps := le.eng.WorkingMemory().FindByClass("step")
	if len(steps) == 0 {
		return StepStats{}, fmt.Errorf("no step WME found")
	}

	modWME, err := le.eng.Modify(steps[0].Timetag, map[string]model.Value{
		"phase": model.NewSymbol("emit"),
	})
	if err != nil {
		return StepStats{}, fmt.Errorf("failed to initiate step: %w", err)
	}
	le.stepTimetag = modWME.Timetag

	// 2. Run engine until quiescence
	startTime := time.Now()
	initialCycles := le.eng.CycleCount()

	_, err = le.eng.Run(0)
	if err != nil {
		return StepStats{}, fmt.Errorf("error running life step: %w", err)
	}

	elapsed := time.Since(startTime)
	cyclesFired := le.eng.CycleCount() - initialCycles
	le.totalCycles = le.eng.CycleCount()
	le.generation++
	le.lastStepMs = float64(elapsed.Microseconds()) / 1000.0

	// Retrieve updated step timetag (was modified by finish-step rule)
	stepWMEs := le.eng.WorkingMemory().FindByClass("step")
	if len(stepWMEs) > 0 {
		le.stepTimetag = stepWMEs[0].Timetag
	}

	liveCells := le.getLiveCellsLocked()

	return StepStats{
		Generation:  le.generation,
		LiveCount:   len(liveCells),
		StepCycles:  cyclesFired,
		TotalCycles: le.totalCycles,
		StepTimeMs:  le.lastStepMs,
		WMECount:    le.eng.WorkingMemory().Count(),
		Cells:       liveCells,
	}, nil
}

// Stats returns current simulation snapshot metrics.
func (le *LifeEngine) Stats() StepStats {
	le.mu.RLock()
	defer le.mu.RUnlock()

	liveCells := le.getLiveCellsLocked()
	return StepStats{
		Generation:  le.generation,
		LiveCount:   len(liveCells),
		StepCycles:  0,
		TotalCycles: le.totalCycles,
		StepTimeMs:  le.lastStepMs,
		WMECount:    le.eng.WorkingMemory().Count(),
		Cells:       liveCells,
	}
}

// LoadPattern clears the grid and spawns a classic Life pattern at (startR, startC).
func (le *LifeEngine) LoadPattern(name string, startR, startC int) error {
	le.Clear()

	le.mu.Lock()
	defer le.mu.Unlock()

	var offsets [][2]int

	switch strings.ToLower(name) {
	case "blinker":
		offsets = [][2]int{{0, 0}, {0, 1}, {0, 2}}
	case "toad":
		offsets = [][2]int{
			{0, 1}, {0, 2}, {0, 3},
			{1, 0}, {1, 1}, {1, 2},
		}
	case "beacon":
		offsets = [][2]int{
			{0, 0}, {0, 1},
			{1, 0}, {1, 1},
			{2, 2}, {2, 3},
			{3, 2}, {3, 3},
		}
	case "pulsar":
		// Classic period-3 oscillator
		offsets = pulsarOffsets()
	case "glider":
		offsets = [][2]int{
			{0, 1},
			{1, 2},
			{2, 0}, {2, 1}, {2, 2},
		}
	case "lwss": // Lightweight Spaceship
		offsets = [][2]int{
			{0, 1}, {0, 4},
			{1, 0},
			{2, 0}, {2, 4},
			{3, 0}, {3, 1}, {3, 2}, {3, 3},
		}
	case "gosper": // Gosper Glider Gun
		offsets = gosperGunOffsets()
	case "acorn": // Methuselah: lives 5206 generations
		offsets = [][2]int{
			{0, 1},
			{1, 3},
			{2, 0}, {2, 1}, {2, 4}, {2, 5}, {2, 6},
		}
	case "r-pentomino":
		offsets = [][2]int{
			{0, 1}, {0, 2},
			{1, 0}, {1, 1},
			{2, 1},
		}
	case "random":
		// 15% random fill
		count := (le.width * le.height) / 7
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		occupied := make(map[[2]int]bool)
		for i := 0; i < count; i++ {
			row := r.Intn(le.height)
			col := r.Intn(le.width)
			coord := [2]int{row, col}
			if !occupied[coord] {
				occupied[coord] = true
				le.eng.Make("cell", map[string]model.Value{
					"r":     model.NewInt(int64(row)),
					"c":     model.NewInt(int64(col)),
					"alive": model.NewInt(1),
				})
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown pattern name: %s", name)
	}

	for _, offset := range offsets {
		r := ((startR+offset[0])%le.height + le.height) % le.height
		c := ((startC+offset[1])%le.width + le.width) % le.width
		le.eng.Make("cell", map[string]model.Value{
			"r":     model.NewInt(int64(r)),
			"c":     model.NewInt(int64(c)),
			"alive": model.NewInt(1),
		})
	}

	return nil
}

func pulsarOffsets() [][2]int {
	var res [][2]int
	rows := []int{1, 6, 8, 13}
	cols1 := []int{3, 4, 5}
	cols2 := []int{9, 10, 11}

	for _, r := range rows {
		for _, c := range cols1 {
			res = append(res, [2]int{r, c})
		}
		for _, c := range cols2 {
			res = append(res, [2]int{r, c})
		}
	}

	cols := []int{1, 6, 8, 13}
	rows1 := []int{3, 4, 5}
	rows2 := []int{9, 10, 11}

	for _, c := range cols {
		for _, r := range rows1 {
			res = append(res, [2]int{r, c})
		}
		for _, r := range rows2 {
			res = append(res, [2]int{r, c})
		}
	}
	return res
}

func gosperGunOffsets() [][2]int {
	return [][2]int{
		{5, 1}, {5, 2}, {6, 1}, {6, 2}, // Left square
		{5, 11}, {6, 11}, {7, 11},
		{4, 12}, {8, 12},
		{3, 13}, {9, 13},
		{3, 14}, {9, 14},
		{6, 15},
		{4, 16}, {8, 16},
		{5, 17}, {6, 17}, {7, 17},
		{6, 18},
		{3, 21}, {4, 21}, {5, 21},
		{3, 22}, {4, 22}, {5, 22},
		{2, 23}, {6, 23},
		{1, 25}, {2, 25}, {6, 25}, {7, 25},
		{3, 35}, {4, 35}, {3, 36}, {4, 36}, // Right square
	}
}
