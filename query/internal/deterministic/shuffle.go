// Package deterministic provides reproducible ordering operations shared by
// Query use cases whose model inputs must remain stable for the same seed.
package deterministic

const (
	stateSize = 624
	period    = 397
)

// randomSource retains the pseudo-random sequence state used only by seeded
// in-memory shuffles; it does not generate or identify domain objects.
type randomSource struct {
	// state is the fixed 624-word MT state initialized from one request seed.
	state [stateSize]uint32
	// index selects the next word and triggers regeneration at stateSize.
	index int
}

func newRandomSource(seed uint64) *randomSource {
	key := []uint32{uint32(seed)}
	if high := uint32(seed >> 32); high != 0 {
		key = append(key, high)
	}
	result := &randomSource{}
	result.seedByArray(key)
	return result
}

func (g *randomSource) seed(value uint32) {
	g.state[0] = value
	for index := 1; index < stateSize; index++ {
		previous := g.state[index-1]
		g.state[index] = 1812433253*(previous^(previous>>30)) + uint32(index)
	}
	g.index = stateSize
}

func (g *randomSource) seedByArray(key []uint32) {
	g.seed(19650218)
	stateIndex, keyIndex := 1, 0
	iterations := stateSize
	if len(key) > iterations {
		iterations = len(key)
	}
	for ; iterations > 0; iterations-- {
		previous := g.state[stateIndex-1]
		g.state[stateIndex] = (g.state[stateIndex] ^ ((previous ^ (previous >> 30)) * 1664525)) +
			key[keyIndex] + uint32(keyIndex)
		stateIndex++
		keyIndex++
		if stateIndex >= stateSize {
			g.state[0] = g.state[stateSize-1]
			stateIndex = 1
		}
		if keyIndex >= len(key) {
			keyIndex = 0
		}
	}
	for iterations = stateSize - 1; iterations > 0; iterations-- {
		previous := g.state[stateIndex-1]
		g.state[stateIndex] = (g.state[stateIndex] ^ ((previous ^ (previous >> 30)) * 1566083941)) -
			uint32(stateIndex)
		stateIndex++
		if stateIndex >= stateSize {
			g.state[0] = g.state[stateSize-1]
			stateIndex = 1
		}
	}
	g.state[0] = 0x80000000
}

func (g *randomSource) uint32() uint32 {
	if g.index >= stateSize {
		for index := 0; index < stateSize; index++ {
			next := (g.state[index] & 0x80000000) |
				(g.state[(index+1)%stateSize] & 0x7fffffff)
			g.state[index] = g.state[(index+period)%stateSize] ^ (next >> 1)
			if next&1 != 0 {
				g.state[index] ^= 0x9908b0df
			}
		}
		g.index = 0
	}

	value := g.state[g.index]
	g.index++
	value ^= value >> 11
	value ^= (value << 7) & 0x9d2c5680
	value ^= (value << 15) & 0xefc60000
	value ^= value >> 18
	return value
}

func (g *randomSource) below(limit int) int {
	bits := bitLength(limit)
	for {
		value := int(g.uint32() >> (32 - bits))
		if value < limit {
			return value
		}
	}
}

func bitLength(value int) uint {
	var result uint
	for current := value; current > 0; current >>= 1 {
		result++
	}
	return result
}

// Shuffle reorders values in place. The same initial values and seed always
// produce the same order so Report batching remains reproducible.
func Shuffle[T any](values []T, seed uint64) {
	random := newRandomSource(seed)
	for index := len(values) - 1; index > 0; index-- {
		other := random.below(index + 1)
		values[index], values[other] = values[other], values[index]
	}
}
