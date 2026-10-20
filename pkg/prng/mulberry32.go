package prng

import (
	"math"
)

// Mulberry32 implements the 32-bit PRNG used in FogSense simulation.
type Mulberry32 struct {
	state uint32
}

// NewMulberry32 creates a new Mulberry32 PRNG with the given seed.
func NewMulberry32(seed uint32) *Mulberry32 {
	return &Mulberry32{state: seed}
}

// Next returns a pseudo-random float64 in [0, 1).
func (m *Mulberry32) Next() float64 {
	m.state += 0x6d2b79f5
	t := (m.state ^ (m.state >> 15)) * (1 | m.state)
	t = (t + ((t ^ (t >> 7)) * (61 | t))) ^ t
	val := t ^ (t >> 14)
	return float64(val) / 4294967296.0
}

// NextGaussian returns a Gaussian-distributed random number with mean 0 and stddev 1.
func (m *Mulberry32) NextGaussian() float64 {
	var u, v float64
	for u == 0 {
		u = m.Next()
	}
	for v == 0 {
		v = m.Next()
	}
	return math.Sqrt(-2*math.Log(u)) * math.Cos(2*math.Pi*v)
}
