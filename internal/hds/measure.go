package hds

import "math"

// Measurements of one trace. Nil fields could not be measured.
type Measurements struct {
	Max    *float64 `json:"max"`
	Min    *float64 `json:"min"`
	Pp     *float64 `json:"pp"`
	Avg    *float64 `json:"avg"`
	Rms    *float64 `json:"rms"`
	Freq   *float64 `json:"freq"`
	Period *float64 `json:"period"`
	Duty   *float64 `json:"duty"`
	// Clipped is set when part of the trace is beyond the screen edge, so
	// the amplitude values are lower than the real ones.
	Clipped bool `json:"clipped"`
}

// Measure computes measurements from samples in volts taken dt seconds apart.
// raw holds the matching screen positions, used to detect clipping.
func Measure(v []float64, raw []int, dt float64) Measurements {
	var m Measurements
	if len(v) < 2 {
		return m
	}
	lo, hi, sum, sq := v[0], v[0], 0.0, 0.0
	for _, x := range v {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
		sum += x
		sq += x * x
	}
	n := float64(len(v))
	m.Max, m.Min, m.Pp = ptr(hi), ptr(lo), ptr(hi-lo)
	m.Avg, m.Rms = ptr(sum/n), ptr(math.Sqrt(sq/n))
	for _, r := range raw {
		if r >= DivsY/2*DivPixels || r <= -DivsY/2*DivPixels {
			m.Clipped = true
			break
		}
	}

	// Period from rising crossings of the mid level, with hysteresis so that
	// noise on a slow edge does not count as extra crossings. A trace needs
	// a swing of a few sample counts to be measured at all.
	if dt <= 0 || len(raw) != len(v) || rawSwing(raw) < 4 {
		return m
	}
	// The device interleaves two ADCs, and next to a fast edge one sample
	// may land on the other side of it; a median of three removes those.
	v = median3(v)
	mid, hyst := (hi+lo)/2, (hi-lo)*0.1
	var rises []float64
	state := 0
	for i, x := range v {
		switch {
		case x > mid+hyst && state <= 0:
			if state < 0 {
				rises = append(rises, crossing(v, i, mid))
			}
			state = 1
		case x < mid-hyst && state >= 0:
			state = -1
		}
	}
	if len(rises) < 2 {
		return m
	}
	first, last := rises[0], rises[len(rises)-1]
	period := (last - first) / float64(len(rises)-1) * dt
	m.Period, m.Freq = ptr(period), ptr(1/period)
	// Duty over whole periods only, so a partial period does not skew it.
	a, b, high := int(math.Ceil(first)), int(last), 0
	if b <= a {
		return m
	}
	for _, x := range v[a : b+1] {
		if x > mid {
			high++
		}
	}
	m.Duty = ptr(100 * float64(high) / float64(b-a+1))
	return m
}

// crossing returns the fractional index where v passes level before index i.
func crossing(v []float64, i int, level float64) float64 {
	for j := i; j > 0; j-- {
		a, b := v[j-1], v[j]
		if a <= level && b > level {
			return float64(j-1) + (level-a)/(b-a)
		}
	}
	return float64(i)
}

func median3(v []float64) []float64 {
	out := make([]float64, len(v))
	copy(out, v)
	for i := 1; i < len(v)-1; i++ {
		a, b, c := v[i-1], v[i], v[i+1]
		out[i] = math.Max(math.Min(a, b), math.Min(math.Max(a, b), c))
	}
	return out
}

func rawSwing(raw []int) int {
	lo, hi := raw[0], raw[0]
	for _, r := range raw {
		lo, hi = min(lo, r), max(hi, r)
	}
	return hi - lo
}

func ptr(f float64) *float64 { return &f }
