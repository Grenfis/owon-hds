package app

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Averaging of successive frames. The device offers only sample and peak
// modes over USB (:ACQuire:MODE SAMPle|PEAK), so the loop averages the traces
// it reads. The measurements, the spectrum, the math channel and the CSV all
// see the averaged trace, which is also finer than the 25 counts of a
// division. Sweeps read their own frames and are not averaged here.

// SetAverage sets how many frames the scope view averages; 1 turns it off.
func (a *App) SetAverage(n int) error {
	if n < 1 || n > 256 {
		return fmt.Errorf("усреднение: %d кадров вне 1…256", n)
	}
	a.avgN.Store(int32(n))
	return nil
}

// averager keeps the mean of the frames so far and, once there are n of
// them, an exponential average over about n frames, as scopes do: it
// follows a slow change without keeping a window of frames. Frames that
// differ in what places a trace on the screen start it again.
type averager struct {
	key   string
	count int
	acc   [][]float64
}

func (v *averager) reset() { *v = averager{} }

// apply replaces the traces of f with the average, including f, of n frames.
func (v *averager) apply(f *Frame, n int) {
	if key := frameKey(f); key != v.key {
		*v = averager{key: key, acc: make([][]float64, len(f.Channels))}
	}
	v.count++
	w := 1 / float64(min(v.count, n))
	for i := range f.Channels {
		ch := &f.Channels[i]
		acc := v.acc[i]
		if len(acc) != len(ch.Raw) {
			acc = slices.Clone(ch.Raw)
		} else {
			for j, x := range ch.Raw {
				acc[j] += (x - acc[j]) * w
			}
		}
		v.acc[i] = acc
		// A hundredth of a count is far below the noise and keeps the
		// frame short in JSON.
		for j, x := range acc {
			ch.Raw[j] = math.Round(x*100) / 100
		}
	}
	f.Averaged = min(v.count, n)
}

func frameKey(f *Frame) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %g %s %s", f.TimeScale, f.HOffset, f.DepMem, f.AcqMode)
	for _, ch := range f.Channels {
		fmt.Fprintf(&b, "|%s %t %g %d %s %d", ch.Name, ch.On, ch.VoltsPerDiv, ch.Offset, ch.Coupling, len(ch.Raw))
	}
	return b.String()
}
