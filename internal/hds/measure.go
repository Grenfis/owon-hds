package hds

import (
	"math"
	"math/cmplx"
)

// Measurements of one trace. Nil fields could not be measured.
type Measurements struct {
	Max *float64 `json:"max"`
	Min *float64 `json:"min"`
	Pp  *float64 `json:"pp"`
	// Top and Base are the flat levels of a pulse: the most common level in
	// the upper and the lower half of the trace. A trace without flat parts,
	// such as a sine or a triangle, has them at Max and Min.
	Top  *float64 `json:"top"`
	Base *float64 `json:"base"`
	Amp  *float64 `json:"amp"` // Top − Base
	// Overshoot and Preshoot are how far Max rises above Top and Min falls
	// below Base, in percent of Amp.
	Overshoot *float64 `json:"overshoot"`
	Preshoot  *float64 `json:"preshoot"`
	Avg       *float64 `json:"avg"`
	Rms       *float64 `json:"rms"`
	// The cycle values cover whole periods only, so that a part of a period
	// on the screen does not skew them. AcRms leaves the mean out.
	CycAvg *float64 `json:"cycAvg"`
	CycRms *float64 `json:"cycRms"`
	AcRms  *float64 `json:"acRms"`
	Freq   *float64 `json:"freq"`
	Period *float64 `json:"period"`
	Duty   *float64 `json:"duty"` // percent of the time above the 50 % level
	Rise   *float64 `json:"rise"` // from 10 % to 90 % of Amp
	Fall   *float64 `json:"fall"` // from 90 % to 10 % of Amp
	PWidth *float64 `json:"pWidth"`
	NWidth *float64 `json:"nWidth"`
	// Clipped is set when part of the trace is beyond the screen edge, so
	// the amplitude values are lower than the real ones.
	Clipped bool `json:"clipped"`

	// For Relate: the samples and the crossings of the 50 % level as
	// fractional sample indexes.
	v            []float64
	rises, falls []float64
}

// Measure computes measurements from samples in volts taken dt seconds apart.
// raw holds the matching screen positions, used to detect clipping and to
// find the flat levels.
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
	m.v = v
	m.Max, m.Min, m.Pp = ptr(hi), ptr(lo), ptr(hi-lo)
	m.Avg, m.Rms = ptr(sum/n), ptr(math.Sqrt(sq/n))
	for _, r := range raw {
		if r >= DivsY/2*DivPixels || r <= -DivsY/2*DivPixels {
			m.Clipped = true
			break
		}
	}

	// Levels and times need a swing of a few sample counts.
	if len(raw) != len(v) || rawSwing(raw) < 4 {
		return m
	}
	top, base := levels(v, raw)
	amp := top - base
	m.Top, m.Base, m.Amp = ptr(top), ptr(base), ptr(amp)
	m.Overshoot, m.Preshoot = ptr(100*(hi-top)/amp), ptr(100*(base-lo)/amp)
	if dt <= 0 {
		return m
	}

	// Crossings of the 50 % level, with hysteresis so that noise on a slow
	// edge does not count as extra crossings. The device interleaves two
	// ADCs, and next to a fast edge one sample may land on the other side of
	// it; a median of three removes those.
	s := median3(v)
	mid, hyst := base+amp/2, amp*0.1
	state := 0
	for i, x := range s {
		switch {
		case x > mid+hyst && state <= 0:
			if state < 0 {
				m.rises = append(m.rises, crossing(s, i, mid))
			}
			state = 1
		case x < mid-hyst && state >= 0:
			if state > 0 {
				m.falls = append(m.falls, crossing(s, i, mid))
			}
			state = -1
		}
	}
	lo10, hi90 := base+amp*0.1, base+amp*0.9
	m.Rise = edgeTime(s, m.rises, m.falls, lo10, hi90, dt)
	m.Fall = edgeTime(s, m.falls, m.rises, hi90, lo10, dt)
	m.PWidth = gap(m.rises, m.falls, dt)
	m.NWidth = gap(m.falls, m.rises, dt)
	if len(m.rises) < 2 {
		return m
	}
	first, last := m.rises[0], m.rises[len(m.rises)-1]
	period := (last - first) / float64(len(m.rises)-1) * dt
	m.Period, m.Freq = ptr(period), ptr(1/period)

	// Whole periods only.
	a, b := int(math.Round(first)), int(math.Round(last))
	if b <= a {
		return m
	}
	var high int
	sum, sq = 0, 0
	for i, x := range v[a:b] {
		sum += x
		sq += x * x
		if s[a+i] > mid {
			high++
		}
	}
	n = float64(b - a)
	avg, rms := sum/n, math.Sqrt(sq/n)
	m.CycAvg, m.CycRms = ptr(avg), ptr(rms)
	m.AcRms = ptr(math.Sqrt(math.Max(0, rms*rms-avg*avg)))
	m.Duty = ptr(100 * float64(high) / n)
	return m
}

// levels returns the top and the base of a trace: the most common count in
// its upper and lower half, if at least a fifth of that half's samples sit
// on it. Otherwise the trace has no flat level there, and it is the extreme.
// Noise of one count splits a flat level over two neighbouring counts, so a
// neighbour with a quarter of the mode's samples or more is averaged in.
func levels(v []float64, raw []int) (top, base float64) {
	ilo, ihi := 0, 0
	for i, r := range raw {
		if r < raw[ilo] {
			ilo = i
		}
		if r > raw[ihi] {
			ihi = i
		}
	}
	rlo, rhi := raw[ilo], raw[ihi]
	cnt := make([]int, rhi-rlo+1)
	for _, r := range raw {
		cnt[r-rlo]++
	}
	// v is linear in raw; this turns a level in counts back into volts.
	volts := func(r float64) float64 {
		return v[ilo] + (r-float64(rlo))*(v[ihi]-v[ilo])/float64(rhi-rlo)
	}
	mid := float64(rlo+rhi) / 2
	level := func(from, step int) float64 {
		total, best, at := 0, 0, from
		for r := from; (float64(r)-mid)*float64(step) < 0; r += step {
			c := cnt[r-rlo]
			total += c
			if c > best {
				best, at = c, r
			}
		}
		if best*5 < total {
			return volts(float64(from))
		}
		sum, w := float64(at*best), best
		for _, r := range []int{at - 1, at + 1} {
			if r >= rlo && r <= rhi && 4*cnt[r-rlo] >= best {
				sum += float64(r * cnt[r-rlo])
				w += cnt[r-rlo]
			}
		}
		return volts(sum / float64(w))
	}
	return level(rhi, -1), level(rlo, 1)
}

// crossing returns the fractional index where v passes level before index i,
// in either direction.
func crossing(v []float64, i int, level float64) float64 {
	for j := i; j > 0; j-- {
		a, b := v[j-1], v[j]
		if (a <= level) != (b <= level) {
			return float64(j-1) + (level-a)/(b-a)
		}
	}
	return float64(i)
}

// edgeTime returns the mean time v takes from level from to level to over
// the edges at the given 50 % crossings. The search for an edge's ends stops
// at the neighbouring crossings of the other direction, and an edge cut by
// them or by the screen edge does not count.
func edgeTime(v, edges, others []float64, from, to, dt float64) *float64 {
	up := to > from
	past := func(x, level float64) bool { return (x >= level) == up }
	var sum float64
	var count int
	for _, e := range edges {
		lo, hi := 0, len(v)-1
		for _, o := range others {
			if o < e {
				lo = max(lo, int(math.Ceil(o)))
			} else {
				hi = min(hi, int(o))
				break
			}
		}
		j := int(e)
		for j > lo && past(v[j], from) {
			j--
		}
		k := int(e) + 1
		for k < hi && !past(v[k], to) {
			k++
		}
		if past(v[j], from) || !past(v[k], to) || j+1 >= len(v) || k < 1 {
			continue
		}
		t0 := float64(j) + (from-v[j])/(v[j+1]-v[j])
		t1 := float64(k-1) + (to-v[k-1])/(v[k]-v[k-1])
		sum += t1 - t0
		count++
	}
	if count == 0 {
		return nil
	}
	return ptr(sum / float64(count) * dt)
}

// gap returns the mean time from each crossing in a to the next one in b.
func gap(a, b []float64, dt float64) *float64 {
	var sum float64
	var count int
	for _, x := range a {
		for _, y := range b {
			if y > x {
				sum += y - x
				count++
				break
			}
		}
	}
	if count == 0 {
		return nil
	}
	return ptr(sum / float64(count) * dt)
}

// Pair relates the second channel to the first.
type Pair struct {
	// Gain and Phase compare the fundamentals, as on a Bode plot: noise, DC
	// and harmonics hardly change them. Phase is in degrees from −180 to
	// 180 and negative when the second channel lags.
	Gain  *float64 `json:"gain"`
	Phase *float64 `json:"phase"`
	// Delay is the time from a rising edge of the first channel to the
	// nearest one of the second, within half a period either way.
	Delay *float64 `json:"delay"`
}

// Relate compares b to a. period is the period of a; dt is the sample step.
func Relate(a, b Measurements, period, dt float64) Pair {
	var p Pair
	// A second channel of another frequency has no fixed phase to the first.
	sameFreq := b.Period == nil || period <= 0 || math.Abs(*b.Period/period-1) <= 0.05
	if n := len(a.rises); n >= 2 && len(b.v) == len(a.v) && sameFreq {
		// The fundamental over the whole periods of a. The second channel
		// needs no edges of its own, so a signal deep in the noise still has
		// a gain and a phase.
		first, last := a.rises[0], a.rises[n-1]
		i0, i1 := int(math.Round(first)), int(math.Round(last))
		w := 2 * math.Pi * float64(n-1) / (last - first)
		fa, fb := fundamental(a.v, i0, i1, w), fundamental(b.v, i0, i1, w)
		if cmplx.Abs(fa) > 0 {
			r := fb / fa
			p.Gain, p.Phase = ptr(cmplx.Abs(r)), ptr(cmplx.Phase(r)*180/math.Pi)
		}
	}
	if p.Gain == nil && a.Pp != nil && b.Pp != nil && *a.Pp > 0 {
		p.Gain = ptr(*b.Pp / *a.Pp)
	}
	if period <= 0 || dt <= 0 || len(a.rises) == 0 || len(b.rises) == 0 || !sameFreq {
		return p
	}
	// A circular mean, so that delays near half a period either way do not
	// cancel out.
	var c, s float64
	for _, x := range a.rises {
		best := b.rises[0]
		for _, y := range b.rises {
			if math.Abs(y-x) < math.Abs(best-x) {
				best = y
			}
		}
		ang := 2 * math.Pi * (best - x) * dt / period
		c += math.Cos(ang)
		s += math.Sin(ang)
	}
	p.Delay = ptr(math.Atan2(s, c) / (2 * math.Pi) * period)
	return p
}

// Response is the gain and phase of one channel against another at a known
// frequency, as a frequency response sweep measures them.
type Response struct {
	Gain  float64
	Phase float64 // degrees, negative when the second channel lags
	// Freq is the frequency of the first channel from its rising edges on
	// the screen, so that a screen taken at another frequency shows.
	Freq float64
}

// Respond takes the fundamentals of a and b at f over the whole periods of f
// on the screen. With the frequency known, b needs no edges of its own, so
// the output of a filter deep in its stopband still has a gain and a phase.
// It returns false when the screen holds fewer than two periods.
func Respond(a, b Measurements, f, dt float64) (Response, bool) {
	var r Response
	n := len(a.v)
	if n == 0 || len(b.v) != n || f <= 0 || dt <= 0 {
		return r, false
	}
	periods := math.Floor(float64(n) * dt * f)
	if periods < 2 {
		return r, false
	}
	m := min(n, int(math.Round(periods/(f*dt))))
	w := 2 * math.Pi * f * dt
	fa, fb := fundamental(a.v, 0, m, w), fundamental(b.v, 0, m, w)
	if cmplx.Abs(fa) == 0 {
		return r, false
	}
	g := fb / fa
	r.Gain, r.Phase = cmplx.Abs(g), cmplx.Phase(g)*180/math.Pi
	if k := len(a.rises); k >= 2 {
		r.Freq = float64(k-1) / ((a.rises[k-1] - a.rises[0]) * dt)
	}
	return r, true
}

// fundamental returns the complex amplitude at w radians per sample of
// v[i0:i1] less its mean.
func fundamental(v []float64, i0, i1 int, w float64) complex128 {
	var mean float64
	for _, x := range v[i0:i1] {
		mean += x
	}
	mean /= float64(i1 - i0)
	var sum complex128
	for i := i0; i < i1; i++ {
		sum += complex(v[i]-mean, 0) * cmplx.Exp(complex(0, -w*float64(i)))
	}
	return sum
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
