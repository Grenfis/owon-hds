// Package sim emulates an HDS272S at the SCPI level, so the application can
// be used and tested without the device. CH1 sees the generator output, CH2
// sees the same signal after an RC low-pass filter with a 10 kHz corner.
// Reply formats follow an HDS272S with firmware V8.8.2.
package sim

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"owon-hds/internal/hds"
)

const (
	points   = hds.DivsX * hds.DivSamples
	filterFc = 10e3
)

type channel struct {
	display  bool
	coupling string
	probe    float64
	scale    float64 // volts per division as shown, probe included
	offset   float64 // divisions
}

type generator struct {
	wave     string
	freq     float64
	amp      float64 // Vpp
	offset   float64
	duty     float64 // %
	symmetry float64 // %
	on       bool
}

// Device implements hds.Transport.
type Device struct {
	mu       sync.Mutex
	ch       [2]channel
	tdiv     float64
	tdivStr  string
	hoffset  float64 // divisions; positive moves the trigger point left
	trigSrc  int
	trigEdge string  // RISe or FALl
	trigLvl  float64 // volts
	sweep    string  // AUTo, NORMal or SINGle
	trigCpl  string
	running  bool
	acqMode  string
	depmem   string
	gen      generator
	screen   string // OSC, DMM or AG
	dmmFunc  string
	dmmRel   bool
	last     [2][]int
	trigged  bool
	busy     time.Duration // delay of the next waveform query after a scale change or autoset
	pending  []byte
	// Latency is added to every waveform query to mimic the real device.
	Latency time.Duration
}

func New() *Device {
	return &Device{
		ch: [2]channel{
			{display: true, coupling: "DC", probe: 1, scale: 0.5},
			{display: true, coupling: "DC", probe: 1, scale: 0.5},
		},
		tdiv: 200e-6, tdivStr: "200us",
		trigEdge: "RISe", sweep: "AUTo", trigCpl: "DC",
		running: true, acqMode: "SAMPle", depmem: "4K",
		gen:     generator{wave: "SINe", freq: 1e3, amp: 2, duty: 50, symmetry: 50, on: true},
		screen:  "OSC",
		dmmFunc: "DCV",
		Latency: 20 * time.Millisecond,
	}
}

func (d *Device) Close() error { return nil }

func (d *Device) Write(p []byte) (int, error) {
	cmd := strings.TrimSpace(string(p))
	reply, delay := d.handle(cmd)
	if delay > 0 {
		time.Sleep(delay)
	}
	d.mu.Lock()
	d.pending = append(d.pending, reply...)
	d.mu.Unlock()
	return len(p), nil
}

func (d *Device) Read(p []byte, timeout time.Duration) (int, error) {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		time.Sleep(timeout)
		return 0, hds.ErrTimeout
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	d.mu.Unlock()
	return n, nil
}

// keywords in SCPI notation: the upper-case part is the short form.
var keywords = strings.Fields(`CH1 CH2 DISPlay COUPling PROBe SCALe OFFSet HORizontal ACQuire
	MODe DEPMem DATa WAVe SCReen HEAD TRIGger STATus SINGle EDGe LEVel SOURce SWEep FUNCtion
	FREQuency PERiod AMPLitude HIGHt LOW SYMMetry WIDTh DTYCycle RISing FALing LOAD CHANnel
	RUNNing AUToseton MODel MEASurement MAX MIN PKPK VAMP AVERage RMS DMM CONFigure VOLTage
	CURRent REL RANGe AUTo MEAS HOLD PULSe RAMP`)

var shortForm = func() map[string]string {
	m := map[string]string{}
	for _, k := range keywords {
		short := shortName(k)
		m[strings.ToUpper(k)], m[short] = short, short
	}
	return m
}()

// normalize turns ":HORIzontal:Scale" into "HOR:SCAL".
func normalize(header string) string {
	toks := strings.Split(strings.Trim(strings.ToUpper(header), ":"), ":")
	for i, t := range toks {
		if s, ok := shortForm[t]; ok {
			toks[i] = s
		}
	}
	return strings.Join(toks, ":")
}

func (d *Device) handle(cmd string) (reply []byte, delay time.Duration) {
	header, arg, _ := strings.Cut(cmd, " ")
	arg = strings.TrimSpace(arg)
	query := strings.HasSuffix(header, "?")
	path := normalize(strings.TrimSuffix(header, "?"))
	// :MODE is a keyword of its own, not the long form of :MODel.
	if strings.EqualFold(strings.Trim(header, ":?"), "MODE") {
		path = "MODE"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if query {
		switch path {
		case "DAT:WAV:SCR:HEAD":
			return d.data(d.head()), d.Latency + d.settle()
		case "DAT:WAV:SCR:CH1", "DAT:WAV:SCR:CH2":
			return d.data(d.samples(int(path[len(path)-1] - '1'))), d.Latency*5/4 + d.settle()
		}
		s, ok := d.query(path)
		if !ok {
			return nil, 0
		}
		return []byte(s + "\n"), 0
	}
	d.set(path, arg)
	return nil, 0
}

// settle returns the extra delay of a waveform query after a scale change
// or autoset, as the device has it.
func (d *Device) settle() time.Duration {
	t := d.busy
	d.busy = 0
	return t
}

func (d *Device) data(payload []byte) []byte {
	out := make([]byte, 4, 4+len(payload))
	binary.LittleEndian.PutUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

func (d *Device) query(path string) (string, bool) {
	sci := func(f float64) string { return strconv.FormatFloat(f, 'e', 6, 64) }
	short := func(f float64) string { return strconv.FormatFloat(f, 'e', -1, 64) }
	onOff := func(b bool) string {
		if b {
			return "ON"
		}
		return "OFF"
	}
	if c, rest, ok := d.chPath(path); ok {
		switch rest {
		case "DISP":
			return onOff(c.display), true
		case "COUP":
			return c.coupling, true
		case "PROB":
			return fmt.Sprintf("%gX", c.probe), true
		case "SCAL":
			return short(c.scale), true
		case "OFFS":
			return strconv.FormatFloat(c.offset, 'f', 2, 64), true
		}
	}
	switch path {
	case "*IDN":
		return "OWON,HDS272S,SIMULATOR,V8.8.2", true
	case "MOD":
		return "HDS272S_1", true
	case "MODE":
		return d.screen, true
	case "HOR:SCAL":
		return short(d.tdiv), true
	case "HOR:OFFS":
		return strconv.FormatFloat(d.hoffset, 'f', 2, 64), true
	case "ACQ:MOD":
		return d.acqMode, true
	case "ACQ:DEPM":
		return d.depmem, true
	case "TRIG:STAT":
		return d.status(), true
	case "TRIG:SING:EDG:SOUR", "TRIG:EDG:SOUR", "TRIG:SOUR":
		return fmt.Sprintf("CH%d", d.trigSrc+1), true
	case "TRIG:SING:EDG", "TRIG:EDG":
		return d.trigEdge, true
	case "TRIG:SING:EDG:LEV", "TRIG:EDG:LEV":
		return fmtVolts(d.trigLvl), true
	case "TRIG:SWE":
		return d.sweep, true
	case "TRIG:COUP":
		return d.trigCpl, true
	case "RUNN":
		if d.running {
			return "RUN", true
		}
		return "STOP", true
	case "FUNC":
		return d.gen.wave, true
	case "FUNC:FREQ":
		return sci(d.gen.freq), true
	case "FUNC:PER":
		return sci(1 / d.gen.freq), true
	case "FUNC:AMPL":
		return sci(d.gen.amp), true
	case "FUNC:OFFS":
		return sci(d.gen.offset), true
	case "FUNC:HIGH":
		return sci(d.gen.offset + d.gen.amp/2), true
	case "FUNC:LOW":
		return sci(d.gen.offset - d.gen.amp/2), true
	case "FUNC:DTYC":
		return strconv.FormatFloat(d.gen.duty, 'f', 1, 64), true
	case "FUNC:SYMM":
		return strconv.FormatFloat(d.gen.symmetry, 'f', 1, 64), true
	case "CHAN":
		return onOff(d.gen.on), true
	case "DMM:CONF":
		return d.dmmFunc, true
	case "DMM:MEAS":
		return d.dmmReading(), true
	}
	return "", false
}

func (d *Device) chPath(path string) (*channel, string, bool) {
	for i, name := range []string{"CH1:", "CH2:"} {
		if rest, ok := strings.CutPrefix(path, name); ok {
			return &d.ch[i], rest, true
		}
	}
	return nil, "", false
}

func (d *Device) set(path, arg string) {
	up := strings.ToUpper(arg)
	num := func(lo, hi float64) (float64, bool) {
		f, err := strconv.ParseFloat(arg, 64)
		if err != nil || f < lo || f > hi {
			return 0, false
		}
		return f, true
	}
	if path == "MODE" {
		if up == "OSC" || up == "DMM" || up == "AG" {
			d.screen = up
		}
		return
	}
	if c, rest, ok := d.chPath(path); ok {
		switch rest {
		case "DISP":
			c.display = up == "ON"
		case "COUP":
			if up == "DC" || up == "AC" || up == "GND" {
				c.coupling = up
			}
		case "PROB":
			if p := hds.ProbeFactor(up); p >= 1 {
				c.scale *= p / c.probe
				c.probe = p
			}
		case "SCAL":
			if v := hds.ParseUnit(arg); v > 0 {
				c.scale = v
			}
			d.busy = 1100 * time.Millisecond
		case "OFFS":
			if v, ok := num(-8, 8); ok {
				c.offset = quantize(v)
			}
		}
		return
	}
	switch path {
	case "HOR:SCAL":
		if v := hds.ParseUnit(arg); v > 0 {
			d.tdiv, d.tdivStr = v, arg
		}
	case "HOR:OFFS":
		if v, ok := num(-1e4, 1e4); ok {
			d.hoffset = quantize(v)
		}
	case "ACQ:MOD":
		d.acqMode = arg
	case "ACQ:DEPM":
		d.depmem = up
	case "TRIG:SING:EDG:SOUR", "TRIG:EDG:SOUR", "TRIG:SOUR":
		if up == "CH1" || up == "CH2" {
			d.trigSrc = int(up[2] - '1')
		}
	case "TRIG:SING:EDG", "TRIG:EDG":
		switch up {
		case "RISE", "RIS":
			d.trigEdge = "RISe"
		case "FALL", "FAL":
			d.trigEdge = "FALl"
		}
	case "TRIG:SING:EDG:LEV", "TRIG:EDG:LEV":
		// The device rounds the level to a whole count of the source channel.
		step := d.ch[d.trigSrc].scale / hds.DivPixels
		d.trigLvl = math.Round(hds.ParseUnit(arg)/step) * step
	case "TRIG:SWE":
		if sw, ok := map[string]string{"AUTO": "AUTo", "AUT": "AUTo", "NORMAL": "NORMal", "NORM": "NORMal",
			"SINGLE": "SINGle", "SING": "SINGle"}[up]; ok {
			d.sweep = sw
			if sw == "SINGle" {
				d.running = true
			}
		}
	case "TRIG:COUP":
		if up == "DC" || up == "AC" {
			d.trigCpl = up
		}
	case "RUNN":
		d.running = up == "RUN"
	case "AUT":
		// Like the device, ignore the command without an argument.
		if arg != "" {
			d.autoset()
			d.busy = 1700 * time.Millisecond
		}
	case "FUNC":
		// Like the device, choosing a waveform turns the output on.
		if w, ok := waves[up]; ok {
			d.gen.wave = w
			d.gen.on = true
		}
	case "FUNC:FREQ":
		if v, ok := num(0.1, 25e6); ok {
			d.gen.freq = v
		}
	case "FUNC:PER":
		if v, ok := num(4e-8, 10); ok {
			d.gen.freq = 1 / v
		}
	case "FUNC:AMPL":
		if v, ok := num(0.01, 5); ok {
			d.gen.amp = v
		}
	case "FUNC:OFFS":
		if v, ok := num(-2.5, 2.5); ok {
			d.gen.offset = v
		}
	case "FUNC:DTYC":
		if v, ok := num(1, 99); ok {
			d.gen.duty = v
		}
	case "FUNC:SYMM":
		if v, ok := num(0, 100); ok {
			d.gen.symmetry = v
		}
	case "CHAN":
		d.gen.on = up == "ON" || up == "1"
	case "DMM:CONF":
		d.dmmFunc = map[string]string{"RES": "RES", "RESISTANCE": "RES", "DIOD": "DIOD", "DIODE": "DIOD",
			"CONT": "CONT", "CONTINUITY": "CONT", "CAP": "CAP", "CAPACITANCE": "CAP"}[up]
	case "DMM:CONF:VOLT":
		d.dmmFunc = up + "V"
	case "DMM:CONF:CURR":
		d.dmmFunc = up + "A"
	case "DMM:REL":
		d.dmmRel = up == "ON"
	}
}

func (d *Device) status() string {
	switch {
	case !d.running:
		return "STOP"
	case d.trigged:
		return "TRIG"
	case d.sweep == "AUTo":
		return "AUTo"
	}
	return "READY"
}

// autoset picks scales so that the generator signal fills the screen.
func (d *Device) autoset() {
	pp := d.gen.amp + 2*math.Abs(d.gen.offset)
	for i := range d.ch {
		d.ch[i].scale = pick125(pp/6, d.ch[i].probe*0.01)
		d.ch[i].offset = 0
	}
	t := pick125(2.5/d.gen.freq/hds.DivsX, 5e-9)
	d.tdiv, d.tdivStr = t, fmtSeconds(t)
	d.trigSrc, d.trigLvl, d.hoffset = 0, d.gen.offset, 0
}

// head acquires a new frame, so that the channel queries that follow it
// return data from the same moment, and describes it.
func (d *Device) head() []byte {
	if d.running || d.last[0] == nil {
		d.acquire()
	}
	type ch struct {
		NAME      string
		DISPLAY   string
		COUPLING  string
		PROBE     string
		SCALE     string
		OFFSET    int
		FREQUENCE float64
	}
	onOff := map[bool]string{true: "ON", false: "OFF"}
	chans := make([]ch, 2)
	for i, c := range d.ch {
		chans[i] = ch{
			NAME: fmt.Sprintf("CH%d", i+1), DISPLAY: onOff[c.display], COUPLING: c.coupling,
			PROBE: fmt.Sprintf("%gX", c.probe), SCALE: fmtVolts(c.scale),
			OFFSET: int(math.Round(c.offset * hds.DivPixels)),
		}
	}
	h := map[string]any{
		"TIMEBASE": map[string]any{"SCALE": d.tdivStr, "HOFFSET": int(math.Round(d.hoffset * hds.DivPixels))},
		"SAMPLE": map[string]any{"FULLSCREEN": points, "SLOWMOVE": -1, "DATALEN": points,
			"SAMPLERATE": fmtRate(points / (d.tdiv * hds.DivsX)), "TYPE": d.acqMode, "DEPMEM": d.depmem},
		"CHANNEL":   chans,
		"DATATYPE":  "SCREEN",
		"RUNSTATUS": d.status(),
		"IDN":       "owon_v1.2",
		"MODEL":     "HDS272S_1",
		"Trig": map[string]any{"Mode": "SINGle", "Type": "Edge", "Items": map[string]any{
			"Channel": fmt.Sprintf("CH%d", d.trigSrc+1), "Level": fmtVolts(d.trigLvl),
			"Edge": d.trigEdge, "Coupling": d.trigCpl, "Sweep": d.sweep,
		}},
	}
	b, _ := json.Marshal(h)
	return b
}

// samples returns one channel of the frame acquired by the last head query.
func (d *Device) samples(idx int) []byte {
	if d.last[idx] == nil {
		d.acquire()
	}
	raw := d.last[idx]
	out := make([]byte, len(raw))
	for i, r := range raw {
		out[i] = byte(int8(r))
	}
	return out
}

func (d *Device) acquire() {
	dt := d.tdiv * hds.DivsX / points
	// Acquire three screens of signal starting at a random moment, then cut
	// out the screen around the trigger crossing.
	n := 3 * points
	t0 := rand.Float64() * 10 / d.gen.freq
	in := make([]float64, n)
	for i := range in {
		in[i] = d.genValue(t0 + float64(i)*dt)
	}
	sig := [2][]float64{in, lowPass(in, d, t0, dt)}
	for i, c := range d.ch {
		switch c.coupling {
		case "GND":
			sig[i] = make([]float64, n)
		case "AC":
			sig[i] = minusMean(sig[i])
		}
	}

	center := n / 2
	d.trigged = false
	src := sig[d.trigSrc]
	for k := 0; k < points; k++ {
		for _, j := range []int{center + k, center - k} {
			if j < 1 || j >= n {
				continue
			}
			a, b := src[j-1], src[j]
			if (d.trigEdge == "RISe" && a < d.trigLvl && b >= d.trigLvl) ||
				(d.trigEdge == "FALl" && a > d.trigLvl && b <= d.trigLvl) {
				center, d.trigged = j, true
				break
			}
		}
		if d.trigged {
			break
		}
	}
	if !d.trigged && d.sweep != "AUTo" && d.last[0] != nil {
		return // normal and single sweeps wait for a trigger
	}
	if d.trigged && d.sweep == "SINGle" {
		d.running = false
	}
	start := center - points/2 + int(math.Round(d.hoffset*hds.DivSamples))
	start = max(0, min(n-points, start))
	for i, c := range d.ch {
		raw := make([]int, points)
		for k := range raw {
			pos := sig[i][start+k]/c.scale*hds.DivPixels + c.offset*hds.DivPixels + rand.NormFloat64()*0.35
			raw[k] = int(max(-128, min(127, math.Round(pos))))
		}
		d.last[i] = raw
	}
}

// waves maps the long and short forms of a wave name to the spelling the
// device replies with.
var waves = func() map[string]string {
	m := map[string]string{}
	for _, w := range strings.Fields("SINe SQUare RAMP PULSe AmpALT AttALT StairDn StairUD StairUp Besselj Bessely Sinc") {
		m[strings.ToUpper(w)] = w
		m[shortName(w)] = w
	}
	return m
}()

func shortName(k string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return -1
		}
		return r
	}, k)
}

func (d *Device) genValue(t float64) float64 {
	g := d.gen
	if !g.on {
		return 0
	}
	ph := math.Mod(t*g.freq, 1)
	a := g.amp / 2
	var v float64
	switch g.wave {
	case "SQUare":
		v = sign(ph < 0.5)
	case "PULSe":
		v = sign(ph < g.duty/100)
	case "RAMP":
		s := g.symmetry / 100
		switch {
		case s <= 0:
			v = 1 - 2*ph
		case ph < s:
			v = -1 + 2*ph/s
		default:
			v = 1 - 2*(ph-s)/(1-s)
		}
	case "Sinc":
		x := (ph - 0.5) * 8 * math.Pi
		v = 1
		if x != 0 {
			v = math.Sin(x) / x
		}
	default:
		v = math.Sin(2 * math.Pi * ph)
	}
	return g.offset + a*v
}

// lowPass runs the generator signal through a first-order RC filter.
func lowPass(in []float64, d *Device, t0, dt float64) []float64 {
	tau := 1 / (2 * math.Pi * filterFc)
	out := make([]float64, len(in))
	if dt > 50*tau {
		copy(out, in)
		return out
	}
	h := tau / 20
	y := d.genValue(t0 - 10*tau)
	// The input is taken in the middle of each step: at its start the
	// output would lag by half a step, a degree at 10 kHz and 1 µs steps.
	for t := t0 - 10*tau; t < t0; t += h {
		y += (d.genValue(t+h/2) - y) * (1 - math.Exp(-h/tau))
	}
	steps := min(100, max(1, int(math.Ceil(dt/h))))
	hs := dt / float64(steps)
	k := 1 - math.Exp(-hs/tau)
	for i := range in {
		out[i] = y
		for s := 0; s < steps; s++ {
			y += (d.genValue(t0+float64(i)*dt+(float64(s)+0.5)*hs) - y) * k
		}
	}
	return out
}

func (d *Device) dmmReading() string {
	// The device replies with a bare number, "0.0000e+00", without a unit.
	vals := map[string]float64{
		"DCV": 1.2345, "ACV": 0.7071, "DCA": 0.0123, "ACA": 0.0087,
		"RES": 4702, "DIOD": 0.612, "CONT": 12.3, "CAP": 1.02e-7,
	}
	r, ok := vals[d.dmmFunc]
	if !ok {
		return "0.0000e+00"
	}
	v := r * (1 + rand.NormFloat64()*0.0005)
	if d.dmmRel {
		v -= r
	}
	return strconv.FormatFloat(v, 'e', 4, 64)
}

func minusMean(v []float64) []float64 {
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x - sum/float64(len(v))
	}
	return out
}

func sign(b bool) float64 {
	if b {
		return 1
	}
	return -1
}

// pick125 returns the smallest 1-2-5 step that is at least v and floor.
func pick125(v, floor float64) float64 {
	v = math.Max(v, floor)
	e := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*e >= v*0.999 {
			return m * e
		}
	}
	return 10 * e
}

// fmtVolts prints volts as the device does: "1.00 V", "500mV", "0.00pV".
func fmtVolts(v float64) string {
	switch av := math.Abs(v); {
	case av == 0:
		return "0.00pV"
	case av < 1:
		return fmt3(v*1000) + "mV"
	case av >= 1000:
		return fmt3(v/1000) + "kV"
	}
	return fmt3(v) + " V"
}

// quantize rounds divisions to whole sample counts, as the device does.
func quantize(div float64) float64 {
	return math.Round(div*hds.DivPixels) / hds.DivPixels
}

func fmtSeconds(s float64) string {
	for _, u := range []struct {
		name string
		mult float64
	}{{"ns", 1e-9}, {"us", 1e-6}, {"ms", 1e-3}, {"s", 1}} {
		if s < u.mult*1000 || u.name == "s" {
			x := s / u.mult
			if x >= 100 {
				return strconv.FormatFloat(x, 'f', 0, 64) + u.name
			}
			return strconv.FormatFloat(x, 'f', 1, 64) + u.name
		}
	}
	return ""
}

func fmtRate(r float64) string {
	switch {
	case r >= 1e6:
		return strconv.FormatFloat(r/1e6, 'g', 4, 64) + "MSa/s"
	case r >= 1e3:
		return strconv.FormatFloat(r/1e3, 'g', 4, 64) + "KSa/s"
	}
	return strconv.FormatFloat(r, 'g', 4, 64) + "Sa/s"
}

// fmt3 prints three significant digits: 1.00, 10.0, 100.
func fmt3(x float64) string {
	ax := math.Abs(x)
	switch {
	case ax < 10:
		return strconv.FormatFloat(x, 'f', 2, 64)
	case ax < 100:
		return strconv.FormatFloat(x, 'f', 1, 64)
	}
	return strconv.FormatFloat(x, 'f', 0, 64)
}
