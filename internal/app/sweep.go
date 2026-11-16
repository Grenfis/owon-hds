package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"strconv"
	"strings"
	"time"

	"owon-hds/internal/hds"
)

// A sweep: CH1 sees the input of a circuit and CH2 its output, and each
// point takes the gain and phase of CH2 against CH1 from their fundamentals.
// A frequency response sweep steps the generator through frequencies; a
// distortion sweep steps it through amplitudes at one frequency and also
// takes the harmonics of both channels.

type SweepConfig struct {
	// Mode "amp" steps the amplitude from Start to Stop volts peak to peak
	// at Freq; otherwise the sweep steps the frequency from Start to Stop
	// hertz at Amp.
	Mode      string  `json:"mode"`
	Start     float64 `json:"start"`
	Stop      float64 `json:"stop"`
	PerDecade int     `json:"perDecade"`
	Amp       float64 `json:"amp"`     // generator Vpp
	Freq      float64 `json:"freq"`    // Hz
	Average   int     `json:"average"` // screens per point
}

type SweepPoint struct {
	Freq  float64  `json:"freq"`
	Amp   float64  `json:"amp"`   // generator Vpp
	Gain  *float64 `json:"gain"`  // CH2/CH1; nil when not measured
	Phase *float64 `json:"phase"` // degrees, negative when CH2 lags
	In    *float64 `json:"in"`    // AC RMS of CH1, volts
	Out   *float64 `json:"out"`   // AC RMS of CH2
	// A distortion sweep: the harmonics of CH2 from the 2nd on, each against
	// its fundamental, and the total distortion of CH2 and of CH1 from them.
	Harm  []float64 `json:"harm"`
	Thd   *float64  `json:"thd"`
	ThdIn *float64  `json:"thdIn"`
	Note  string    `json:"note"` // why the point is missing
}

type SweepEvent struct {
	Mode  string      `json:"mode"`  // as in the config
	State string      `json:"state"` // "running", "done", "stopped" or "error"
	Index int         `json:"index"` // points done
	Total int         `json:"total"`
	Freq  float64     `json:"freq"` // the point being measured
	Amp   float64     `json:"amp"`
	Point *SweepPoint `json:"point"`
	Error string      `json:"error"`
}

// The slowest scale the sweep uses fills the screen in 0.6 s and shows four
// periods from about 7 Hz.
const sweepMinFreq = 10

// StartSweep starts a sweep; the loop runs it in place of the usual polling.
func (a *App) StartSweep(cfg SweepConfig) error {
	amp := cfg.Mode == "amp"
	switch {
	case a.currentClient() == nil:
		return errors.New("нет связи с прибором")
	case a.sweeping.Load():
		return errors.New("измерение уже идёт")
	case cfg.PerDecade < 1 || cfg.PerDecade > 50:
		return errors.New("точек на декаду: от 1 до 50")
	case amp && !(cfg.Freq >= sweepMinFreq && cfg.Freq <= 1e6):
		return errors.New("частота: от 10 Гц до 1 МГц")
	case amp && !(cfg.Start >= 0.01 && cfg.Stop <= 5 && cfg.Stop > cfg.Start):
		return errors.New("размах генератора должен быть от 0.01 до 5 В, начальный меньше конечного")
	case !amp && !(cfg.Start >= sweepMinFreq && cfg.Stop <= 25e6 && cfg.Stop > cfg.Start):
		return errors.New("частоты должны быть от 10 Гц до 25 МГц, начальная меньше конечной")
	case !amp && !(cfg.Amp >= 0.01 && cfg.Amp <= 5):
		return errors.New("амплитуда генератора: от 0.01 до 5 В")
	}
	cfg.Average = min(max(cfg.Average, 1), 8)
	a.sweepStop.Store(false)
	a.sweeping.Store(true)
	a.sweepReq.Store(&cfg)
	a.queue.poke()
	return nil
}

// StopSweep ends a sweep after the screen in progress; the settings are
// restored as after a full sweep.
func (a *App) StopSweep() {
	a.sweepStop.Store(true)
	a.queue.poke()
}

// sweepSteps spaces the points evenly on a log scale, rounded to three
// significant digits, ends included.
func sweepSteps(cfg SweepConfig) []float64 {
	decades := math.Log10(cfg.Stop / cfg.Start)
	n := max(2, int(math.Ceil(decades*float64(cfg.PerDecade)-1e-9))+1)
	out := make([]float64, 0, n)
	for i := range n {
		f := cfg.Start * math.Pow(cfg.Stop/cfg.Start, float64(i)/float64(n-1))
		f, _ = strconv.ParseFloat(strconv.FormatFloat(f, 'g', 3, 64), 64)
		if len(out) == 0 || f > out[len(out)-1] {
			out = append(out, f)
		}
	}
	return out
}

// timebases are the device's horizontal scales, 1-2-5 from 2 ns to 1000 s.
var timebases = func() []float64 {
	var out []float64
	for e := -9; e <= 3; e++ {
		for _, m := range []float64{1, 2, 5} {
			if t := m * math.Pow(10, float64(e)); t >= 2e-9*0.999 && t <= 1000*1.001 {
				out = append(out, t)
			}
		}
	}
	return out
}()

// timebaseFor returns the fastest scale that shows at least four periods of
// f, so that each point has whole periods to measure and at least 60 screen
// samples per period. The slowest is 50 ms/div, which still fills the screen
// at once.
func timebaseFor(f float64) float64 {
	for _, t := range timebases {
		if f*hds.DivsX*t >= 4 || t >= 50e-3*0.999 {
			return t
		}
	}
	return 50e-3
}

func closestTimebase(t float64) float64 {
	best := timebases[0]
	for _, x := range timebases {
		if math.Abs(math.Log(x/t)) < math.Abs(math.Log(best/t)) {
			best = x
		}
	}
	return best
}

// timebaseCmd prints a scale as the device takes it: "1.0us", "50ms".
func timebaseCmd(t float64) string {
	unit, mult := "s", 1.0
	switch {
	case t < 1e-6*0.999:
		unit, mult = "ns", 1e-9
	case t < 1e-3*0.999:
		unit, mult = "us", 1e-6
	case t < 0.999:
		unit, mult = "ms", 1e-3
	}
	y := math.Round(t/mult*1000) / 1000
	if y < 10 {
		return strconv.FormatFloat(y, 'f', 1, 64) + unit
	}
	return strconv.FormatFloat(y, 'f', 0, 64) + unit
}

// vdivs are the vertical scales at probe 1X; the device multiplies them by
// the probe factor.
var vdivs = []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10}

// vdivCmd prints a scale as the device takes it: "200mV", "1.00V".
func vdivCmd(v float64) string {
	f3 := func(y float64) string {
		switch {
		case y < 10:
			return strconv.FormatFloat(y, 'f', 2, 64)
		case y < 100:
			return strconv.FormatFloat(y, 'f', 1, 64)
		}
		return strconv.FormatFloat(y, 'f', 0, 64)
	}
	switch {
	case v < 1*0.999:
		return f3(v*1000) + "mV"
	case v >= 1000*0.999:
		return f3(v/1000) + "kV"
	}
	return f3(v) + "V"
}

// rangeCmd returns the scale command that fits a channel to the screen, or
// "" when it fits: a swing of 2 to 7 divisions stays, as each change of the
// scale costs over a second. A clipped trace goes two steps coarser, since
// its real swing is unknown; otherwise the swing should cover up to six
// divisions.
func rangeCmd(ch FrameChannel) string {
	probe := hds.ProbeFactor(ch.Probe)
	cur := 0
	for i, v := range vdivs {
		if math.Abs(math.Log(v*probe/ch.VoltsPerDiv)) < math.Abs(math.Log(vdivs[cur]*probe/ch.VoltsPerDiv)) {
			cur = i
		}
	}
	want := cur
	switch {
	case ch.Meas.Clipped:
		want = min(cur+2, len(vdivs)-1)
	case ch.Meas.Pp == nil:
		return ""
	default:
		if divs := *ch.Meas.Pp / ch.VoltsPerDiv; divs >= 2 && divs <= 7 {
			return ""
		}
		want = len(vdivs) - 1
		for i, v := range vdivs {
			if *ch.Meas.Pp/(v*probe) <= 6 {
				want = i
				break
			}
		}
	}
	if want == cur {
		return ""
	}
	return fmt.Sprintf(":%s:SCALe %s", ch.Name, vdivCmd(vdivs[want]*probe))
}

// send sends a setting and shows it in the console log.
func (a *App) send(c *hds.Client, cmd string) error {
	if err := c.Send(cmd); err != nil {
		return err
	}
	a.emit("log", LogEntry{Dir: "out", Text: cmd, Time: time.Now().UnixMilli()})
	return nil
}

// errStopped ends a sweep that the user stopped.
var errStopped = errors.New("остановлено")

// runSweep measures every point and restores the settings it changed. It
// returns only errors of the connection.
func (a *App) runSweep(ctx context.Context, c *hds.Client, cfg SweepConfig) error {
	defer a.sweeping.Store(false)
	amp := cfg.Mode == "amp"
	steps := sweepSteps(cfg)
	ev := func(e SweepEvent) {
		e.Mode, e.Total = cfg.Mode, len(steps)
		a.emit("sweep", e)
	}
	fail := func(err error) error {
		ev(SweepEvent{State: "error", Error: err.Error()})
		return err
	}

	// The settings to restore, from the current screen and the generator.
	var f0 *Frame
	var err error
	for range 3 {
		if f0, err = a.frame(c); err == nil {
			break
		}
	}
	if err != nil {
		return fail(err)
	}
	if len(f0.Channels) != 2 {
		return fail(errors.New("в заголовке прибора нет двух каналов"))
	}
	restore := restoreCmds(c, f0)

	first := cfg.Amp
	if amp {
		first = steps[0]
	}
	setup := []string{
		":FUNCtion SINE", ":FUNCtion:AMPLitude " + formatNumber(first), ":FUNCtion:OFFSet 0", ":CHANnel ON",
		":CH1:DISPlay ON", ":CH2:DISPlay ON", ":CH1:OFFSet 0", ":CH2:OFFSet 0",
		":TRIGger:SINGle:EDGe:SOURce CH1", ":TRIGger:SWEep AUTO", ":TRIGger:SINGle:EDGe:LEVel 0.0mv", ":RUNning RUN",
	}
	curTB := 0.0
	if amp {
		curTB = timebaseFor(cfg.Freq)
		setup = append(setup, ":FUNCtion:FREQuency "+formatNumber(cfg.Freq), ":HORizontal:SCALe "+timebaseCmd(curTB))
	}
	// The input swing is known: up to six divisions of CH1 from the start.
	probe := hds.ProbeFactor(f0.Channels[0].Probe)
	for _, v := range vdivs {
		if first/(v*probe) <= 6 {
			setup = append(setup, ":CH1:SCALe "+vdivCmd(v*probe))
			break
		}
	}
	for _, cmd := range setup {
		if err := a.send(c, cmd); err != nil {
			return fail(err)
		}
	}

	state := "done"
	for i, x := range steps {
		f, vpp, tol := x, cfg.Amp, 0.03
		if amp {
			f, vpp = cfg.Freq, x
		}
		ev(SweepEvent{State: "running", Index: i, Freq: f, Amp: vpp})
		if amp {
			if i > 0 {
				if err := a.send(c, ":FUNCtion:AMPLitude "+formatNumber(vpp)); err != nil {
					return fail(err)
				}
			}
		} else {
			if tb := timebaseFor(f); tb != curTB {
				if err := a.send(c, ":HORizontal:SCALe "+timebaseCmd(tb)); err != nil {
					return fail(err)
				}
				curTB = tb
			}
			if err := a.send(c, ":FUNCtion:FREQuency "+formatNumber(f)); err != nil {
				return fail(err)
			}
			// Neighbouring points must not pass for each other.
			if i+1 < len(steps) {
				tol = min(tol, (steps[i+1]/f-1)/2)
			}
		}
		p, err := a.measurePoint(ctx, c, f, tol, curTB, cfg.Average, amp)
		if errors.Is(err, errStopped) {
			state = "stopped"
			break
		}
		if err != nil {
			return fail(err)
		}
		p.Amp = vpp
		ev(SweepEvent{State: "running", Index: i + 1, Freq: f, Amp: vpp, Point: &p})
	}

	for _, cmd := range restore {
		if err := a.send(c, cmd); err != nil {
			return fail(err)
		}
	}
	ev(SweepEvent{State: state, Index: len(steps)})
	return nil
}

// measurePoint waits for screens of the new setting and averages their gain
// and phase, and with harm the harmonics of both channels. The harmonics are
// turned to the phase of the fundamental of CH1 on their screen, h times for
// the h-th, so that they add up from screen to screen while the noise in
// them averages out. The first screen after the generator or a scale changed
// may have been taken before, so it does not count; nor does a screen whose
// CH1 shows another frequency.
func (a *App) measurePoint(ctx context.Context, c *hds.Client, f, tol, timebase float64, average int, harm bool) (SweepPoint, error) {
	p := SweepPoint{Freq: f}
	screen := time.Duration(timebase * hds.DivsX * float64(time.Second))
	deadline := time.Now().Add(max(8*time.Second, 10*screen))
	var sum complex128
	var in, out float64
	var fund [2]float64    // the sum of the fundamentals' amplitudes
	var hs [2][]complex128 // the sum of the turned harmonics
	n, misses, skip := 0, 0, true
	for n < average {
		if ctx.Err() != nil || a.sweepStop.Load() {
			return p, errStopped
		}
		if time.Now().After(deadline) {
			p.Note = "на CH1 нет сигнала этой частоты"
			break
		}
		fr, err := a.frame(c)
		if err != nil {
			if misses++; misses >= 3 {
				return p, err
			}
			continue
		}
		misses = 0
		a.emit("frame", fr)
		changed := false
		for _, ch := range fr.Channels[:min(2, len(fr.Channels))] {
			if cmd := rangeCmd(ch); cmd != "" {
				if err := a.send(c, cmd); err != nil {
					return p, err
				}
				changed = true
			}
		}
		if changed || skip {
			skip = changed
			continue
		}
		if len(fr.Channels) < 2 || len(fr.Channels[0].Raw) == 0 {
			continue
		}
		ch1, ch2 := fr.Channels[0].Meas, fr.Channels[1].Meas
		dt := fr.TimeDiv * hds.DivsX / float64(len(fr.Channels[0].Raw))
		r, ok := hds.Respond(ch1, ch2, f, dt)
		if !ok || math.Abs(r.Freq/f-1) > tol {
			continue
		}
		if harm {
			var turn complex128
			for k, m := range []hds.Measurements{ch1, ch2} {
				a1, h, _ := hds.Harmonics(m, f, dt)
				if k == 0 {
					turn = cmplx.Conj(a1) / complex(cmplx.Abs(a1), 0)
				}
				fund[k] += cmplx.Abs(a1)
				if hs[k] == nil {
					hs[k] = make([]complex128, len(h))
				}
				for j := range min(len(h), len(hs[k])) {
					hs[k][j] += h[j] * cmplx.Pow(turn, complex(float64(j+2), 0))
				}
			}
		}
		sum += cmplx.Rect(r.Gain, r.Phase*math.Pi/180)
		if ch1.AcRms != nil {
			in += *ch1.AcRms
		}
		if ch2.AcRms != nil {
			out += *ch2.AcRms
		}
		n++
	}
	if n > 0 {
		g := sum / complex(float64(n), 0)
		p.Gain, p.Phase = ptr(cmplx.Abs(g)), ptr(cmplx.Phase(g)*180/math.Pi)
		p.In, p.Out = ptr(in/float64(n)), ptr(out/float64(n))
		if harm && fund[0] > 0 && fund[1] > 0 {
			var thd [2]float64
			for k := range hs {
				for _, h := range hs[k] {
					r := cmplx.Abs(h) / fund[k]
					thd[k] += r * r
					if k == 1 {
						p.Harm = append(p.Harm, r)
					}
				}
			}
			p.ThdIn, p.Thd = ptr(math.Sqrt(thd[0])), ptr(math.Sqrt(thd[1]))
		}
	}
	return p, nil
}

// headSweeps maps the trigger sweep as the frame header names it to the
// command argument.
var headSweeps = map[string]string{"auto": "AUTO", "normal": "NORMal", "norm": "NORMal", "single": "SINGle", "sing": "SINGle"}

// restoreCmds returns the commands that put back what a sweep changes.
func restoreCmds(c *hds.Client, f *Frame) []string {
	var cmds []string
	q := func(cmd string) string {
		s, err := c.Query(cmd)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(s)
	}
	if s := q(":FUNCtion?"); s != "" {
		cmds = append(cmds, ":FUNCtion "+s)
	}
	for _, k := range []string{"FREQuency", "AMPLitude", "OFFSet"} {
		if x, err := strconv.ParseFloat(q(":FUNCtion:"+k+"?"), 64); err == nil {
			cmds = append(cmds, ":FUNCtion:"+k+" "+formatNumber(x))
		}
	}
	if s := strings.ToUpper(q(":CHANnel?")); s == "ON" || s == "OFF" {
		cmds = append(cmds, ":CHANnel "+s)
	}
	for _, ch := range f.Channels {
		cmds = append(cmds,
			fmt.Sprintf(":%s:SCALe %s", ch.Name, vdivCmd(ch.VoltsPerDiv)),
			fmt.Sprintf(":%s:OFFSet %.2f", ch.Name, float64(ch.Offset)/hds.DivPixels),
			fmt.Sprintf(":%s:DISPlay %s", ch.Name, map[bool]string{true: "ON", false: "OFF"}[ch.On]))
	}
	if f.TimeDiv > 0 {
		cmds = append(cmds, ":HORizontal:SCALe "+timebaseCmd(closestTimebase(f.TimeDiv)))
	}
	if f.Trig.Source != "" {
		cmds = append(cmds, ":TRIGger:SINGle:EDGe:SOURce "+strings.ToUpper(f.Trig.Source))
	}
	if s, ok := headSweeps[strings.ToLower(f.Trig.Sweep)]; ok {
		cmds = append(cmds, ":TRIGger:SWEep "+s)
	}
	if f.Trig.LevelStr != "" {
		cmds = append(cmds, fmt.Sprintf(":TRIGger:SINGle:EDGe:LEVel %.1fmv", f.Trig.Level*1000))
	}
	if f.Run == "stop" {
		cmds = append(cmds, ":RUNning STOP")
	}
	return cmds
}

func ptr(f float64) *float64 { return &f }
