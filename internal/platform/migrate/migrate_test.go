package migrate

import "testing"

func TestDownRejectsNonPositiveSteps(t *testing.T) {
	for _, steps := range []int{0, -1} {
		t.Run("steps", func(t *testing.T) {
			if err := Down(Config{}, steps); err == nil {
				t.Fatalf("Down(%d) returned nil error", steps)
			}
		})
	}
}
