package life

import (
	"sort"
	"testing"
)

func canonicalCoords(cells [][2]int) [][2]int {
	sorted := make([][2]int, len(cells))
	copy(sorted, cells)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i][0] == sorted[j][0] {
			return sorted[i][1] < sorted[j][1]
		}
		return sorted[i][0] < sorted[j][0]
	})
	return sorted
}

func coordsEqual(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	ca := canonicalCoords(a)
	cb := canonicalCoords(b)
	for i := range ca {
		if ca[i][0] != cb[i][0] || ca[i][1] != cb[i][1] {
			return false
		}
	}
	return true
}

func TestBlinkerOscillation(t *testing.T) {
	le, err := NewLifeEngine(20, 20)
	if err != nil {
		t.Fatalf("failed to create life engine: %v", err)
	}

	err = le.LoadPattern("blinker", 5, 5)
	if err != nil {
		t.Fatalf("failed to load blinker: %v", err)
	}

	initial := le.GetLiveCells()
	expectedInitial := [][2]int{{5, 5}, {5, 6}, {5, 7}}
	if !coordsEqual(initial, expectedInitial) {
		t.Fatalf("expected initial %v, got %v", expectedInitial, initial)
	}

	// Step 1: Horizontal becomes vertical
	s1, err := le.Step()
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}
	expectedGen1 := [][2]int{{4, 6}, {5, 6}, {6, 6}}
	if !coordsEqual(s1.Cells, expectedGen1) {
		t.Fatalf("gen 1 expected %v, got %v", expectedGen1, s1.Cells)
	}
	if s1.StepCycles <= 0 {
		t.Fatalf("expected step cycles > 0, got %d", s1.StepCycles)
	}

	// Step 2: Vertical becomes horizontal again (period 2)
	s2, err := le.Step()
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}
	if !coordsEqual(s2.Cells, expectedInitial) {
		t.Fatalf("gen 2 expected %v, got %v", expectedInitial, s2.Cells)
	}
	if s2.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", s2.Generation)
	}
}

func TestToadOscillation(t *testing.T) {
	le, err := NewLifeEngine(20, 20)
	if err != nil {
		t.Fatalf("failed to create life engine: %v", err)
	}

	err = le.LoadPattern("toad", 5, 5)
	if err != nil {
		t.Fatalf("failed to load toad: %v", err)
	}

	initial := le.GetLiveCells()
	if len(initial) != 6 {
		t.Fatalf("expected 6 live cells for toad, got %d", len(initial))
	}

	// Step 1
	_, err = le.Step()
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}

	// Step 2 (period 2 oscillator)
	s2, err := le.Step()
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}

	if !coordsEqual(s2.Cells, initial) {
		t.Fatalf("toad did not oscillate back to initial state after 2 steps: %v vs %v", s2.Cells, initial)
	}
}

func TestGliderTranslation(t *testing.T) {
	le, err := NewLifeEngine(30, 30)
	if err != nil {
		t.Fatalf("failed to create life engine: %v", err)
	}

	err = le.LoadPattern("glider", 5, 5)
	if err != nil {
		t.Fatalf("failed to load glider: %v", err)
	}

	initial := le.GetLiveCells()
	if len(initial) != 5 {
		t.Fatalf("expected 5 live cells for glider, got %d", len(initial))
	}

	// A glider shifts down 1 and right 1 every 4 generations
	for i := 0; i < 4; i++ {
		_, err = le.Step()
		if err != nil {
			t.Fatalf("glider step %d failed: %v", i+1, err)
		}
	}

	after4 := le.GetLiveCells()
	expectedAfter4 := [][2]int{
		{6, 7},
		{7, 8},
		{8, 6}, {8, 7}, {8, 8},
	}
	if !coordsEqual(after4, expectedAfter4) {
		t.Fatalf("glider after 4 steps expected %v, got %v", expectedAfter4, after4)
	}
}

func TestToggleAndClear(t *testing.T) {
	le, err := NewLifeEngine(10, 10)
	if err != nil {
		t.Fatalf("failed to create life engine: %v", err)
	}

	// Grid starts empty
	if len(le.GetLiveCells()) != 0 {
		t.Fatalf("expected empty grid, got %d cells", len(le.GetLiveCells()))
	}

	// Toggle on (3, 3)
	state := le.ToggleCell(3, 3)
	if !state {
		t.Fatalf("expected cell (3,3) to be alive after first toggle")
	}
	if len(le.GetLiveCells()) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(le.GetLiveCells()))
	}

	// Toggle off (3, 3)
	state = le.ToggleCell(3, 3)
	if state {
		t.Fatalf("expected cell (3,3) to be dead after second toggle")
	}
	if len(le.GetLiveCells()) != 0 {
		t.Fatalf("expected 0 cells after toggle off, got %d", len(le.GetLiveCells()))
	}

	// Set multiple cells and test Clear()
	le.SetCell(1, 1, true)
	le.SetCell(2, 2, true)
	le.SetCell(3, 3, true)
	if len(le.GetLiveCells()) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(le.GetLiveCells()))
	}

	le.Clear()
	if len(le.GetLiveCells()) != 0 {
		t.Fatalf("expected 0 cells after clear, got %d", len(le.GetLiveCells()))
	}
	if le.Stats().Generation != 0 {
		t.Fatalf("expected generation 0 after clear, got %d", le.Stats().Generation)
	}
}
