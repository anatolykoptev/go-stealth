package identitymodel

// Rand is a SplitMix64 generator. It is chosen for being five lines long, so
// the Go and Rust samplers can reproduce each other exactly.
type Rand struct{ state uint64 }

// NewRand returns a generator whose stream is fixed by seed.
func NewRand(seed uint64) *Rand { return &Rand{state: seed} }

// Uint64 returns the next 64 bits.
func (r *Rand) Uint64() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Float64 returns a value in [0, 1) with 53 random bits.
func (r *Rand) Float64() float64 {
	return float64(r.Uint64()>>11) * (1.0 / (1 << 53))
}
