package coordinator

import (
	"DistributedMandelbrot/task"
	"fmt"
	gimage "image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// newIngestTestCoordinator builds a Coordinator that can ingest tasks for imageCount frames of width x height pixels,
// saving the frames to a temporary directory
func newIngestTestCoordinator(t *testing.T, width int, height int, imageCount uint) *Coordinator {
	t.Helper()
	c := newTestCoordinator()
	c.digitCount = 1
	c.imageCount = imageCount
	c.images = make(map[int]imageTask)
	c.pixelCount = uint(width * height)
	c.rectangle = gimage.Rect(0, 0, width, height)
	c.settings.SavePath = t.TempDir()
	c.settings.RunName = "run"
	if err := os.Mkdir(filepath.Join(c.settings.SavePath, c.settings.RunName), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	return c
}

// rowTask returns a finished task with every pixel of one row set to the given color
func rowTask(id uint, imageNumber uint, row uint, width uint, rowColor color.RGBA) task.Task {
	done := task.NewTask(id, imageNumber)
	for column := range width {
		done.AddResult(task.Pixel{Color: rowColor, Column: column, Row: row})
	}
	return done
}

func framePath(c *Coordinator, imageNumber uint) string {
	return filepath.Join(c.settings.SavePath, c.settings.RunName, fmt.Sprintf("%d.jpg", imageNumber))
}

func TestIngestTaskSavesFrameOnceComplete(t *testing.T) {
	c := newIngestTestCoordinator(t, 8, 2, 1)
	red := color.RGBA{R: 255, A: 255}

	c.ingestTask(rowTask(0, 1, 0, 8, red))
	if _, err := os.Stat(framePath(c, 1)); err == nil {
		t.Fatal("frame was saved before all of its pixels arrived")
	}
	if got := c.imagesInProgress.Load(); got != 1 {
		t.Errorf("imagesInProgress = %d, want 1", got)
	}

	c.ingestTask(rowTask(1, 1, 1, 8, red))
	f, err := os.Open(framePath(c, 1))
	if err != nil {
		t.Fatalf("frame was not saved once complete: %s", err)
	}
	defer f.Close()
	saved, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("saved frame is not a valid JPEG: %s", err)
	}
	if saved.Bounds() != c.rectangle {
		t.Errorf("saved frame bounds = %v, want %v", saved.Bounds(), c.rectangle)
	}
	// JPEG is lossy, so only check the color is clearly red
	r, g, b, _ := saved.At(4, 1).RGBA()
	if r>>8 < 200 || g>>8 > 60 || b>>8 > 60 {
		t.Errorf("saved pixel = (%d, %d, %d), want approximately red", r>>8, g>>8, b>>8)
	}

	if len(c.images) != 0 || c.imagesInProgress.Load() != 0 {
		t.Errorf("completed frame should be released, got %d images (%d in progress)", len(c.images), c.imagesInProgress.Load())
	}
	if c.taskIngestedCount.Load() != 2 || c.imageCompletedCount.Load() != 1 {
		t.Errorf("ingested %d tasks and completed %d images, want 2 and 1", c.taskIngestedCount.Load(), c.imageCompletedCount.Load())
	}
}

func TestIngestTaskInterleavedFrames(t *testing.T) {
	c := newIngestTestCoordinator(t, 4, 3, 2)
	blue := color.RGBA{B: 255, A: 255}

	var id uint
	for row := range uint(3) {
		for imageNumber := uint(1); imageNumber <= 2; imageNumber++ {
			c.ingestTask(rowTask(id, imageNumber, row, 4, blue))
			id++
		}
	}

	for imageNumber := uint(1); imageNumber <= 2; imageNumber++ {
		if _, err := os.Stat(framePath(c, imageNumber)); err != nil {
			t.Errorf("frame %d was not saved: %s", imageNumber, err)
		}
	}
	if c.imageCompletedCount.Load() != 2 || c.imagesInProgress.Load() != 0 {
		t.Errorf("completed %d images with %d in progress, want 2 and 0", c.imageCompletedCount.Load(), c.imagesInProgress.Load())
	}
}

func TestStatus(t *testing.T) {
	c := newIngestTestCoordinator(t, 4, 2, 3)
	c.taskGeneratedCount.Add(5)
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	c.ingestTask(rowTask(0, 1, 0, 4, white))
	c.ingestTask(rowTask(1, 1, 1, 4, white))
	c.ingestTask(rowTask(2, 2, 0, 4, white))

	want := "Tasks [Generated: 5] [Ingested: 3] | Images [Completed: 1] [WIP: 1] [Todo: 2]"
	if got := c.status(); got != want {
		t.Errorf("status() = %q, want %q", got, want)
	}
}
