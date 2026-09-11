package mockprtg

import (
	"hash/fnv"
	"math"
	"time"
)

// epoch anchors every deterministic signal so a channel's value depends only
// on (channel key, timestamp) -- never on when the mock server process
// itself happened to start. This keeps values stable and reproducible across
// container restarts, and identical for repeated requests at the same
// timestamp (no flicker on panel refresh).
var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// signalSpec describes a procedurally generated channel value as a small sum
// of sine waves plus an optional linear trend, clamped to a floor/ceiling.
// It deliberately avoids any PRNG: phase() derives a stable per-channel
// offset from a hash of the channel's key so multiple channels of the same
// kind (e.g. two "Ping" sensors) don't move in lockstep.
type signalSpec struct {
	base        float64
	amp1        float64
	period1     time.Duration
	amp2        float64
	period2     time.Duration
	trendPerDay float64 // linear drift per day, may be negative
	floor       float64
	ceiling     float64
}

// constantSignal builds a signalSpec that always evaluates to v, for
// channels that aren't meant to vary (e.g. a sensor's total memory size).
func constantSignal(v float64) *signalSpec {
	return &signalSpec{base: v, floor: v, ceiling: v}
}

// phase derives a stable pseudo-random phase offset in [0, 2π) from a string
// key.
func phase(key string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return float64(h.Sum32()%10000) / 10000 * 2 * math.Pi
}

// evaluate computes the signal's value at time t. key scopes the derived
// phase offset -- callers pass something unique per channel (e.g.
// "<sensorId>.<channelId>") so otherwise-identical signalSpecs don't produce
// identical-looking series.
func (s *signalSpec) evaluate(key string, t time.Time) float64 {
	elapsed := t.Sub(epoch).Seconds()
	ph := phase(key)

	v := s.base
	if s.period1 > 0 {
		v += s.amp1 * math.Sin(2*math.Pi*elapsed/s.period1.Seconds()+ph)
	}
	if s.period2 > 0 {
		v += s.amp2 * math.Sin(2*math.Pi*elapsed/s.period2.Seconds()+ph*1.618)
	}
	if s.trendPerDay != 0 {
		v += s.trendPerDay * (elapsed / 86400)
	}

	if v < s.floor {
		v = s.floor
	}
	if v > s.ceiling {
		v = s.ceiling
	}
	return v
}
