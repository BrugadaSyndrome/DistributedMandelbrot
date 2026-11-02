package coordinator

import (
	"DistributedMandelbrot/mandelbrot"
	"DistributedMandelbrot/task"
	"bytes"
	gimage "image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// encodeFrame returns a frame of the given size encoded as a JPEG
func encodeFrame(t *testing.T, bounds gimage.Rectangle) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, gimage.NewRGBA(bounds), nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func writeFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0644); err != nil {
		t.Fatal(err)
	}
}

// Only frames that fully decode at the run's size count as done; anything else is rendered again
func TestCompletedFrames(t *testing.T) {
	c := newIngestTestCoordinator(t, 16, 8, 6)
	valid := encodeFrame(t, c.rectangle)

	writeFile(t, c.framePath(1), valid)
	writeFile(t, c.framePath(2), valid[:len(valid)/2]) // cut short, e.g. the coordinator was stopped mid write
	// frame 3 was never saved
	writeFile(t, c.framePath(4), encodeFrame(t, gimage.Rect(0, 0, 8, 8))) // a different size than this run renders
	writeFile(t, c.framePath(5), valid)
	writeFile(t, c.framePath(6), nil)
	writeFile(t, c.framePath(6)+".tmp", valid)
	writeFile(t, filepath.Join(c.runFolder(), "notes.txt"), []byte("not a frame"))

	completed := c.completedFrames()
	if len(completed) != 2 || !completed[1] || !completed[5] {
		t.Errorf("completedFrames() = %v, want frames 1 and 5", completed)
	}
}

// newResumeTestCoordinator plans a run of 4 frames, each split into 3 row tasks
func newResumeTestCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	c := newTestCoordinator()
	c.settings.MandelbrotSettings = mandelbrot.Settings{Width: 5, Height: 3}
	c.settings.TaskGeneration = task.Row
	c.settings.TransitionSettings = []transitionSettings{
		{StartX: -0.5, EndX: -0.7, EndY: 0.2, MagnificationStart: 1, MagnificationEnd: 8, MagnificationStep: 2},
	}
	if err := c.planTasks(); err != nil {
		t.Fatalf("planTasks returned error: %s", err)
	}
	if c.imageCount != 4 || c.taskCount != 12 {
		t.Fatalf("planned %d images and %d tasks, want 4 and 12", c.imageCount, c.taskCount)
	}
	return c
}

// generateAll runs generateTasks to completion and returns every task it queued. The queue is drained as tasks arrive,
// since generating more tasks than it holds would otherwise block forever.
func generateAll(t *testing.T, c *Coordinator) []task.Task {
	t.Helper()
	generated := make(chan struct{})
	go func() {
		c.generateTasks()
		close(generated)
	}()

	var tasks []task.Task
	for {
		select {
		case todo := <-c.tasksTodo:
			tasks = append(tasks, todo)
		case <-generated:
			for len(c.tasksTodo) > 0 {
				tasks = append(tasks, <-c.tasksTodo)
			}
			return tasks
		case <-time.After(2 * time.Second):
			t.Fatalf("generateTasks did not finish; %d tasks generated so far", len(tasks))
		}
	}
}

func TestResumeGeneratesOnlyMissingFrames(t *testing.T) {
	c := newResumeTestCoordinator(t)
	c.resumeFrom(map[uint]bool{1: true, 3: true})

	if c.taskCount != 6 {
		t.Errorf("taskCount = %d, want 6 (3 tasks for each of the 2 missing frames)", c.taskCount)
	}
	generated := generateAll(t, c)
	if uint(len(generated)) != c.taskCount {
		t.Fatalf("generated %d tasks but planned %d; the coordinator would never shut down", len(generated), c.taskCount)
	}

	transition := c.settings.TransitionSettings[0]
	tasksPerImage := make(map[uint]int)
	for _, todo := range generated {
		tasksPerImage[todo.ImageNumber]++
		// Frame numbers start at 1 and transition frames at 0. Skipping frames must not shift later frames' positions.
		x, y, magnification := transition.frame(todo.ImageNumber - 1)
		if want := (task.Frame{CenterX: x, CenterY: y, Magnification: magnification}); todo.Frame != want {
			t.Errorf("image %d has frame %+v, want %+v", todo.ImageNumber, todo.Frame, want)
		}
	}
	if len(tasksPerImage) != 2 || tasksPerImage[2] != 3 || tasksPerImage[4] != 3 {
		t.Errorf("tasks per image = %v, want 3 each for images 2 and 4", tasksPerImage)
	}

	// Progress counts the frames finished before the resume
	if !strings.Contains(c.status(), "Images [Completed: 2] [WIP: 0] [Todo: 2]") {
		t.Errorf("status() = %q, want 2 completed and 2 to do", c.status())
	}
}

// When every frame is already saved there is nothing to render, which also covers making the movie later
func TestResumeWithEveryFrameDone(t *testing.T) {
	c := newResumeTestCoordinator(t)
	c.resumeFrom(map[uint]bool{1: true, 2: true, 3: true, 4: true})

	if c.taskCount != 0 {
		t.Errorf("taskCount = %d, want 0", c.taskCount)
	}
	if generated := generateAll(t, c); len(generated) != 0 {
		t.Errorf("generated %d tasks, want none", len(generated))
	}
}

// Frame numbering, and so the file names ffmpeg reads, must not depend on how many frames are already done
func TestResumeKeepsFrameNumbering(t *testing.T) {
	c := newResumeTestCoordinator(t)
	digitCount := c.digitCount
	c.resumeFrom(map[uint]bool{1: true, 2: true, 3: true})
	if c.digitCount != digitCount || c.imageCount != 4 {
		t.Errorf("resumeFrom changed digitCount/imageCount to %d/%d, want %d/4", c.digitCount, c.imageCount, digitCount)
	}
}

func TestSaveFrameLeavesOnlyTheFinishedFile(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "7.jpg")
	writeFile(t, path, []byte("an older, broken copy of the frame"))

	frame := gimage.NewRGBA(gimage.Rect(0, 0, 4, 3))
	if err := saveFrame(path, frame); err != nil {
		t.Fatalf("saveFrame returned error: %s", err)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "7.jpg" {
		t.Errorf("folder holds %v, want only 7.jpg", entries)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("saved frame does not decode: %s", err)
	}
	if saved.Bounds() != frame.Bounds() {
		t.Errorf("saved frame bounds = %v, want %v", saved.Bounds(), frame.Bounds())
	}
}

func TestSaveFrameReportsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing folder", "1.jpg")
	if err := saveFrame(path, gimage.NewRGBA(gimage.Rect(0, 0, 1, 1))); err == nil {
		t.Error("saveFrame should fail when the folder does not exist")
	}
}

// A save that fails part way must leave the previous file untouched rather than a damaged frame under the final name
func TestSaveFrameFailureKeepsExistingFile(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "1.jpg")
	previous := encodeFrame(t, gimage.Rect(0, 0, 4, 3))
	writeFile(t, path, previous)

	tooWide := gimage.NewRGBA(gimage.Rect(0, 0, 1<<16, 1)) // jpeg.Encode refuses images this wide
	if err := saveFrame(path, tooWide); err == nil {
		t.Fatal("saveFrame should fail for an image JPEG cannot hold")
	}

	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, previous) {
		t.Errorf("the existing frame was changed by a failed save (err: %v)", err)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Errorf("folder holds %v, want only the existing frame", entries)
	}
}
