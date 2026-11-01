package task

import (
	"testing"
)

type pixelKey struct {
	row, column uint
}

// assertCoverage checks that coordinates cover every pixel of a height x width image exactly once
func assertCoverage(t *testing.T, coordinates []Coordinate, height uint, width uint) {
	t.Helper()
	seen := make(map[pixelKey]int, len(coordinates))
	for _, c := range coordinates {
		if c.Row >= height || c.Column >= width {
			t.Fatalf("coordinate (row %d, column %d) is outside a %dx%d image", c.Row, c.Column, height, width)
		}
		seen[pixelKey{c.Row, c.Column}]++
	}
	if uint(len(seen)) != height*width {
		t.Fatalf("covered %d of %d pixels", len(seen), height*width)
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("pixel (row %d, column %d) covered %d times", k.row, k.column, n)
		}
	}
}

func TestAddTasksForRow(t *testing.T) {
	var all []Coordinate
	for row := range uint(4) {
		tk := NewTask(0, 1)
		tk.AddTasksForRow(-0.5, 0.25, 3, row, 7)
		if len(tk.Tasks) != 7 {
			t.Fatalf("row %d: got %d coordinates, want 7", row, len(tk.Tasks))
		}
		for _, c := range tk.Tasks {
			if c.Row != row {
				t.Fatalf("row %d task contains coordinate for row %d", row, c.Row)
			}
		}
		all = append(all, tk.Tasks...)
	}
	assertCoverage(t, all, 4, 7)
}

func TestAddTasksForColumn(t *testing.T) {
	var all []Coordinate
	for column := range uint(7) {
		tk := NewTask(0, 1)
		tk.AddTasksForColumn(-0.5, 0.25, 3, 4, column)
		for _, c := range tk.Tasks {
			if c.Column != column {
				t.Fatalf("column %d task contains coordinate for column %d", column, c.Column)
			}
		}
		all = append(all, tk.Tasks...)
	}
	assertCoverage(t, all, 4, 7)
}

func TestAddTasksForImage(t *testing.T) {
	tk := NewTask(0, 1)
	tk.AddTasksForImage(-0.5, 0.25, 3, 4, 7)
	assertCoverage(t, tk.Tasks, 4, 7)
}

func TestAddTasksForImageByGrid(t *testing.T) {
	sizes := []struct {
		name          string
		height, width uint
	}{
		{"4K", 2160, 3840},
		{"evenly divisible", 100, 200},
		{"with remainder", 107, 253},
		{"smaller than the grid", 5, 7},
	}
	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			var all []Coordinate
			for gridRow := range GridDivisions {
				for gridColumn := range GridDivisions {
					tk := NewTask(0, 1)
					tk.AddTasksForImageByGrid(-0.5, 0.25, 3, size.height, size.width, gridRow, gridColumn)
					if size.height >= GridDivisions && size.width >= GridDivisions && len(tk.Tasks) == 0 {
						t.Fatalf("grid cell (%d, %d) is empty", gridRow, gridColumn)
					}
					all = append(all, tk.Tasks...)
				}
			}
			assertCoverage(t, all, size.height, size.width)
		})
	}
}

func TestAddTasksCopiesFrameParameters(t *testing.T) {
	tk := NewTask(0, 1)
	tk.AddTasksForImage(-0.75, 0.1, 42, 2, 2)
	for _, c := range tk.Tasks {
		if c.CenterX != -0.75 || c.CenterY != 0.1 || c.Magnification != 42 {
			t.Fatalf("coordinate %s does not carry the frame's center and magnification", c.String())
		}
	}
}

func TestGetNextTaskAndAddResult(t *testing.T) {
	tk := NewTask(5, 2)
	tk.AddTasksForRow(0, 0, 1, 3, 4)

	var processed []Coordinate
	for {
		c, err := tk.GetNextTask()
		if err != nil {
			break
		}
		processed = append(processed, c)
		tk.AddResult(Pixel{Row: c.Row, Column: c.Column})
		if len(processed) > len(tk.Tasks) {
			t.Fatal("GetNextTask did not stop after all coordinates were processed")
		}
	}

	if len(processed) != 4 || len(tk.Results) != 4 {
		t.Fatalf("processed %d coordinates and recorded %d results, want 4 of each", len(processed), len(tk.Results))
	}
	for i, c := range processed {
		if c != tk.Tasks[i] {
			t.Errorf("coordinate %d = %s, want %s", i, c.String(), tk.Tasks[i].String())
		}
	}
	if _, err := tk.GetNextTask(); err == nil {
		t.Error("GetNextTask should keep returning an error once all results are in")
	}
}

func TestGetNextTaskOnEmptyTask(t *testing.T) {
	tk := NewTask(0, 1)
	if _, err := tk.GetNextTask(); err == nil {
		t.Error("GetNextTask on a task with no coordinates should return an error")
	}
}

func TestGenerationString(t *testing.T) {
	tests := map[Generation]string{
		Row:    "Row",
		Column: "Column",
		Image:  "Image",
		Grid:   "Grid",
	}
	for g, want := range tests {
		if got := g.String(); got != want {
			t.Errorf("Generation(%d).String() = %q, want %q", int(g), got, want)
		}
	}
}
