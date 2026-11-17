package app

import (
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	for _, c := range []struct{ key, value, want string }{
		{"ch1.scale", "1.00V", ":CH1:SCALe 1.00V"},
		{"ch2.coupling", "ac", ":CH2:COUPling AC"},
		{"gen.freq", "12345.6", ":FUNCtion:FREQuency 12345.6"},
		{"gen.freq", "25000000", ":FUNCtion:FREQuency 25000000"},
		{"gen.wave", "square", ":FUNCtion SQUare"},
		{"trig.level", "-250mv", ":TRIGger:SINGle:EDGe:LEVel -250mv"},
		{"ch1.offset", "-1.24", ":CH1:OFFSet -1.24"},
		{"auto", "", ":AUToseton ON"},
	} {
		got, err := command(c.key, c.value)
		if err != nil || got != c.want {
			t.Errorf("command(%q, %q) = %q, %v; want %q", c.key, c.value, got, err, c.want)
		}
	}
	for _, c := range [][2]string{
		{"gen.freq", "30e6"}, {"ch1.scale", "1V;:SERial X"}, {"gen.wave", "noise"}, {"nope", "1"},
	} {
		if got, err := command(c[0], c[1]); err == nil {
			t.Errorf("command(%q, %q) = %q, want error", c[0], c[1], got)
		}
	}
}

func TestBlocked(t *testing.T) {
	for _, cmd := range []string{":SERial 123", ":SER 1", ":Selfcorrec", ":AUTocalibration", ":AUT",
		":PRACTISE:KEYSET 3", "DOWNload:TXT x", ":VIDEO", ":VOLFULL"} {
		if !Blocked(cmd) {
			t.Errorf("%q not blocked", cmd)
		}
	}
	for _, cmd := range []string{"*IDN?", ":AUToseton", ":SECurity:BRIGhtness?", ":SHUTdown:TIMe?", ":DMM:MEAS?"} {
		if Blocked(cmd) {
			t.Errorf("%q blocked", cmd)
		}
	}
}

// TestSimulatorLoop connects to the simulator, receives frames and checks
// that a generator change reaches the trace.
func TestSimulatorLoop(t *testing.T) {
	var mu sync.Mutex
	var frames []*Frame
	a := New(func(event string, data any) {
		if f, ok := data.(*Frame); ok && event == "frame" {
			mu.Lock()
			frames = append(frames, f)
			mu.Unlock()
		}
	})
	st, err := a.Connect("sim")
	if err != nil || !strings.Contains(st.IDN, "HDS272S") {
		t.Fatalf("connect: %+v %v", st, err)
	}
	defer a.Disconnect()

	waitFreq := func(want float64) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			var f *Frame
			if len(frames) > 0 {
				f = frames[len(frames)-1]
			}
			mu.Unlock()
			if f != nil && f.Channels[0].Meas.Freq != nil && math.Abs(*f.Channels[0].Meas.Freq-want)/want < 0.03 {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("CH1 never measured %g Hz", want)
	}
	waitFreq(1000)
	for _, f := range []string{"1500", "1800", "2000"} { // collapses into the last value
		if err := a.Set("gen.freq", f); err != nil {
			t.Fatal(err)
		}
	}
	waitFreq(2000)

	g, err := a.ReadGen()
	if err != nil || g.Freq == nil || *g.Freq != 2000 || g.Output == nil || !*g.Output {
		t.Fatalf("ReadGen = %+v, %v", g, err)
	}
	r, err := a.Console(":DATa:WAVe:SCReen:HEAD?")
	if err != nil || r.Kind != "data" || !strings.Contains(r.Reply, "TIMEBASE") {
		t.Fatalf("console head = %+v, %v", r, err)
	}
	if _, err := a.Console(":SERial 1"); err == nil {
		t.Fatal(":SERial was not blocked")
	}

	// The device screen follows the view; the console leaves it alone.
	waitScreen := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		var got string
		for time.Now().Before(deadline) {
			if r, err := a.Console(":MODE?"); err == nil {
				if got = r.Reply; got == want {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("screen %q, want %q", got, want)
	}
	waitScreen("OSC")
	a.SetView("dmm")
	waitScreen("DMM")
	a.SetView("console")
	time.Sleep(300 * time.Millisecond)
	waitScreen("DMM")
	a.SetView("scope")
	waitScreen("OSC")
}

// TestSweep measures the simulator's RC filter, 10 kHz to the -3 dB point,
// and checks that the settings come back.
// sweepOnSim runs a sweep on the simulator, checks that it puts the
// settings back and returns its points.
func sweepOnSim(t *testing.T, cfg SweepConfig) []SweepPoint {
	t.Helper()
	events := make(chan SweepEvent, 100)
	a := New(func(event string, data any) {
		if e, ok := data.(SweepEvent); ok && event == "sweep" {
			events <- e
		}
	})
	if _, err := a.Connect("sim"); err != nil {
		t.Fatal(err)
	}
	defer a.Disconnect()
	before := map[string]string{}
	queries := []string{":FUNCtion:FREQuency?", ":FUNCtion:AMPLitude?", ":CH1:SCALe?", ":CH2:SCALe?", ":HORizontal:SCALe?", ":TRIGger:SWEep?"}
	for _, q := range queries {
		r, err := a.Console(q)
		if err != nil {
			t.Fatal(err)
		}
		before[q] = r.Reply
	}

	if err := a.StartSweep(cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.StartSweep(cfg); err == nil {
		t.Error("a second sweep started")
	}
	var points []SweepPoint
	timeout := time.After(60 * time.Second)
	for done := false; !done; {
		select {
		case e := <-events:
			if e.Mode != cfg.Mode {
				t.Errorf("event of mode %q, want %q", e.Mode, cfg.Mode)
			}
			if e.Point != nil {
				points = append(points, *e.Point)
			}
			if e.State == "error" || e.State == "stopped" {
				t.Fatalf("sweep %s: %s", e.State, e.Error)
			}
			done = e.State == "done"
		case <-timeout:
			t.Fatal("sweep did not finish")
		}
	}
	for _, q := range queries {
		r, _ := a.Console(q)
		x, err1 := strconv.ParseFloat(r.Reply, 64)
		y, err2 := strconv.ParseFloat(before[q], 64)
		if r.Reply != before[q] && (err1 != nil || err2 != nil || math.Abs(x/y-1) > 1e-9) {
			t.Errorf("%s = %q after the sweep, was %q", q, r.Reply, before[q])
		}
	}
	return points
}

func TestSweep(t *testing.T) {
	points := sweepOnSim(t, SweepConfig{Start: 1000, Stop: 100e3, PerDecade: 2, Amp: 2, Average: 2})
	if len(points) != 5 {
		t.Fatalf("%d points, want 5", len(points))
	}
	for _, p := range points {
		x := p.Freq / 10e3
		gain, phase := 1/math.Sqrt(1+x*x), -math.Atan(x)*180/math.Pi
		if p.Gain == nil || p.Phase == nil {
			t.Errorf("%g Hz: no result, %s", p.Freq, p.Note)
			continue
		}
		if math.Abs(*p.Gain/gain-1) > 0.01 || math.Abs(*p.Phase-phase) > 0.5 {
			t.Errorf("%g Hz: gain %.3f phase %.1f, want %.3f %.1f", p.Freq, *p.Gain, *p.Phase, gain, phase)
		}
		if p.Thd != nil || p.Harm != nil {
			t.Errorf("%g Hz: harmonics in a frequency sweep", p.Freq)
		}
	}
}

// The simulator is linear: the distortion is what the counts and the noise
// leave. Eight screens averaged with the harmonics turned to the fundamental
// bring it to about 0.2 %, against 0.5 % from one screen.
func TestDistortionSweep(t *testing.T) {
	points := sweepOnSim(t, SweepConfig{Mode: "amp", Start: 0.05, Stop: 5, PerDecade: 2, Freq: 5e3, Average: 8})
	want := []float64{0.05, 0.158, 0.5, 1.58, 5}
	if len(points) != len(want) {
		t.Fatalf("%d points, want %d", len(points), len(want))
	}
	x := 0.5
	gain := 1 / math.Sqrt(1+x*x)
	for i, p := range points {
		if p.Amp != want[i] || p.Freq != 5e3 {
			t.Errorf("point %d at %g V %g Hz, want %g V 5 kHz", i, p.Amp, p.Freq, want[i])
		}
		if p.Gain == nil || p.Thd == nil || p.ThdIn == nil {
			t.Errorf("%g V: no result, %s", p.Amp, p.Note)
			continue
		}
		if math.Abs(*p.Gain/gain-1) > 0.02 {
			t.Errorf("%g V: gain %.3f, want %.3f", p.Amp, *p.Gain, gain)
		}
		if len(p.Harm) != 9 {
			t.Errorf("%g V: %d harmonics, want 9", p.Amp, len(p.Harm))
		}
		if *p.Thd > 0.005 || *p.ThdIn > 0.005 {
			t.Errorf("%g V: distortion %.4f, input %.4f, want under 0.5 %%", p.Amp, *p.Thd, *p.ThdIn)
		}
		t.Logf("%5g V: gain %.3f, distortion %.3f %%, input %.3f %%", p.Amp, *p.Gain, *p.Thd*100, *p.ThdIn*100)
	}
}

// The device turns the generator output on when a waveform is chosen; the
// program keeps it as it was.
func TestWaveKeepsOutput(t *testing.T) {
	a := New(func(string, any) {})
	if _, err := a.Connect("sim"); err != nil {
		t.Fatal(err)
	}
	defer a.Disconnect()
	query := func(q string) string {
		t.Helper()
		r, err := a.Console(q)
		if err != nil {
			t.Fatal(err)
		}
		return r.Reply
	}
	set := func(key, value, q, want string) {
		t.Helper()
		if err := a.Set(key, value); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for !strings.EqualFold(query(q), want) {
			if time.Now().After(deadline) {
				t.Fatalf("%s did not become %s", q, want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	set("gen.output", "OFF", ":CHANnel?", "OFF")
	set("gen.wave", "SQUare", ":FUNCtion?", "SQUARE")
	time.Sleep(200 * time.Millisecond)
	if s := query(":CHANnel?"); s != "OFF" {
		t.Errorf("output %s after a waveform, was off", s)
	}
	set("gen.output", "ON", ":CHANnel?", "ON")
	set("gen.wave", "RAMP", ":FUNCtion?", "RAMP")
	time.Sleep(200 * time.Millisecond)
	if s := query(":CHANnel?"); s != "ON" {
		t.Errorf("output %s after a waveform, was on", s)
	}
}

func TestAverager(t *testing.T) {
	frame := func(scale string, raw ...float64) *Frame {
		return &Frame{TimeScale: scale, Channels: []FrameChannel{{Name: "CH1", On: true, VoltsPerDiv: 1, Raw: raw}, {Name: "CH2"}}}
	}
	var v averager
	// Noise of ±2 counts around 10 and −10: the mean of the frames so far.
	v.apply(frame("1ms", 12, -8), 4)
	f := frame("1ms", 8, -12)
	v.apply(f, 4)
	if first2(f) != [2]float64{10, -10} || f.Averaged != 2 {
		t.Fatalf("two frames: %v, %d frames", first2(f), f.Averaged)
	}
	// Past n frames each new one weighs 1/n.
	for range 3 {
		v.apply(frame("1ms", 10, -10), 4)
	}
	f = frame("1ms", 14, -10)
	v.apply(f, 4)
	if first2(f) != [2]float64{11, -10} || f.Averaged != 4 {
		t.Fatalf("exponential: %v, %d frames", first2(f), f.Averaged)
	}
	// A new time base starts again.
	f = frame("2ms", 3, 4)
	v.apply(f, 4)
	if first2(f) != [2]float64{3, 4} || f.Averaged != 1 {
		t.Fatalf("after a change: %v, %d frames", first2(f), f.Averaged)
	}
	// The measurements follow the average.
	f.measure()
	if m := f.Channels[0].Meas; m.Max == nil || *m.Max != 4.0/25 {
		t.Fatalf("max %v", m.Max)
	}
}

// first2 returns the two samples of CH1 in these tests.
func first2(f *Frame) [2]float64 { return [2]float64{f.Channels[0].Raw[0], f.Channels[0].Raw[1]} }

func TestWriteFile(t *testing.T) {
	a := New(func(string, any) {})
	if _, err := a.WriteFile("a.csv", "eA=="); err == nil {
		t.Fatal("wrote without a folder")
	}
	if _, err := a.ChooseFolder(); err == nil {
		t.Fatal("chose a folder without a dialog")
	}
	dir := t.TempDir()
	a.FolderDialog = func() (string, error) { return dir, nil }
	if d, err := a.ChooseFolder(); err != nil || d != dir {
		t.Fatalf("folder %q, %v", d, err)
	}
	for _, bad := range []string{"../x.csv", `..\x.csv`, "sub/x.csv", "C:x.csv", "", ".."} {
		if _, err := a.WriteFile(bad, "eA=="); err == nil {
			t.Errorf("wrote %q", bad)
		}
	}
	p, err := a.WriteFile("a.csv", "eA==")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Fatalf("%s holds %q", p, b)
	}
}
