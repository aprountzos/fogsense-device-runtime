package prng

import (
	"math"
	"testing"
)

func TestMulberry32(t *testing.T) {
	rng := NewMulberry32(42)
	expected := []float64{
		0.6011037519201636,
		0.44829055899754167,
		0.8524657934904099,
		0.6697340414393693,
	}

	for i, exp := range expected {
		val := rng.Next()
		if math.Abs(val-exp) > 1e-12 {
			t.Errorf("step %d: got %v, expected %v", i, val, exp)
		}
	}
}
