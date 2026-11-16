package hds

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestParseUnit(t *testing.T) {
	for in, want := range map[string]float64{
		"1.0ms": 1e-3, "200us": 200e-6, "2.0ns": 2e-9, "1000s": 1000,
		"10.0mv": 0.01, "1.00v": 1, "2.00kV": 2000, "500uv": 500e-6,
		"1.000000e+04": 1e4, "(500ks/s)": 0, "25mv": 0.025, "-1.5v": -1.5,
		"1.00 v": 1, "480mv": 0.48, "0.00pv": 0, "5e-01": 0.5,
	} {
		if got := ParseUnit(in); math.Abs(got-want) > 1e-12 {
			t.Errorf("ParseUnit(%q) = %g, want %g", in, got, want)
		}
	}
}

func TestParseHead(t *testing.T) {
	// Sample from OWON's SCPI manual.
	h, err := ParseHead([]byte(`{"timebase":{"scale":"1.0ms","hoffset":0},"sample":{"fullscreen":1520,"slowmove":-1,"datalen":1520,"samplerate":"(500ks/s)","type":"sample","depmem":"10k"},"channel":[{"name":"ch1","display":"on","coupling":"ac","probe":"10x","scale":"5.00mv","offset":50,"frequence":0,"inverse":"off"},{"name":"ch2","display":"on","coupling":"ac","probe":"10x","scale":"10.0mv","offset":45,"frequence":0,"inverse":"off"}],"datatype":"screen","runstatus":"auto","trig":{"mode":"single","type":"edge","items":{"channel":"ch1","level":"32.0mv","edge":"rise","coupling":"dc","holdoff":"100ns"},"sweep":"auto"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Channel) != 2 || h.Channel[0].Offset != 50 || h.Sample.DataLen != 1520 || h.Trig.Items.Level != "32.0mv" {
		t.Fatalf("unexpected head: %+v", h)
	}
	if v := h.Channel[0].VoltsPerDiv(); math.Abs(v-0.005) > 1e-12 {
		t.Errorf("VoltsPerDiv = %g, want 0.005", v)
	}
	if h.Sweep() != "auto" {
		t.Errorf("Sweep = %q, want auto", h.Sweep())
	}
}

func TestParseHeadDevice(t *testing.T) {
	// Reply of an HDS272S with firmware V8.8.2, horizontal offset 0.48 div.
	h, err := ParseHead([]byte(`{"TIMEBASE":{"SCALE":"500us","HOFFSET":12},"SAMPLE":{"FULLSCREEN":600,"SLOWMOVE":-1,"DATALEN":600,"SAMPLERATE":"500KSa/s","TYPE":"SAMPle","DEPMEM":"4K"},"CHANNEL":[{"NAME":"CH1","DISPLAY":"ON","COUPLING":"AC","PROBE":"10X","SCALE":"1.00 V","OFFSET":0,"FREQUENCE":1000.00000},{"NAME":"CH2","DISPLAY":"OFF","COUPLING":"DC","PROBE":"10X","SCALE":"500mV","OFFSET":-50,"FREQUENCE":0.00000}],"DATATYPE":"SCREEN","RUNSTATUS":"TRIG","IDN":"owon_v1.2","MODEL":"HDS272S_1","Trig":{"Mode":"SINGle","Type":"Edge","Items":{"Channel":"CH1","Level":"480mV","Edge":"RISe","Coupling":"AC","Sweep":"AUTo"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if v := h.Channel[0].VoltsPerDiv(); v != 1 {
		t.Errorf("CH1 VoltsPerDiv = %g, want 1", v)
	}
	if v := h.Channel[1].VoltsPerDiv(); v != 0.5 {
		t.Errorf("CH2 VoltsPerDiv = %g, want 0.5", v)
	}
	if h.Channel[1].On() || h.Channel[1].Offset != -50 {
		t.Errorf("CH2 = %+v", h.Channel[1])
	}
	if h.Sample.SampleRate != "500KSa/s" {
		t.Errorf("SampleRate = %q, want 500KSa/s", h.Sample.SampleRate)
	}
	if d := h.HOffsetDivs(); math.Abs(d-0.48) > 1e-12 {
		t.Errorf("HOffsetDivs = %g, want 0.48", d)
	}
	if h.Sweep() != "auto" || h.Trig.Items.Edge != "rise" || ParseUnit(h.Trig.Items.Level) != 0.48 {
		t.Errorf("trig = %+v", h.Trig)
	}
}

func TestDecodeSamples(t *testing.T) {
	got := DecodeSamples([]byte{0x28, 0x27, 0xd8, 0xd7, 0x7f, 0x80})
	want := []int{40, 39, -40, -41, 127, -128}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DecodeSamples = %v, want %v", got, want)
		}
	}
}

func TestMeasureSine(t *testing.T) {
	const n, cycles = 300, 4.0
	v := make([]float64, n)
	raw := make([]int, n)
	for i := range v {
		v[i] = 0.5 + 1.0*math.Sin(2*math.Pi*cycles*float64(i)/n)
		raw[i] = int(math.Round(v[i] * 25))
	}
	dt := 1e-3 / n // the screen spans 1 ms, so the sine is 4 kHz
	m := Measure(v, raw, dt)
	if math.Abs(*m.Freq-4000) > 20 {
		t.Errorf("freq = %g, want 4000", *m.Freq)
	}
	if math.Abs(*m.Pp-2) > 0.01 || math.Abs(*m.Avg-0.5) > 0.01 {
		t.Errorf("pp = %g avg = %g", *m.Pp, *m.Avg)
	}
	if math.Abs(*m.Duty-50) > 2 {
		t.Errorf("duty = %g, want 50", *m.Duty)
	}
	if m.Clipped {
		t.Error("clipped set for an on-screen trace")
	}
}

func TestMeasureInterleaveGlitch(t *testing.T) {
	// 1 kHz square over 6 ms, 600 samples, with a one-sample glitch after
	// every rising edge as the device's interleaved ADCs produce.
	const n = 600
	v := make([]float64, n)
	raw := make([]int, n)
	for i := range raw {
		raw[i] = 40
		if i%100 >= 50 {
			raw[i] = -40
		}
		if i%100 == 1 && i > 1 {
			raw[i] = -41
		}
		v[i] = float64(raw[i]) / 25
	}
	m := Measure(v, raw, 6e-3/n)
	if m.Freq == nil || math.Abs(*m.Freq-1000) > 5 {
		t.Errorf("freq = %v, want 1000", m.Freq)
	}
	if m.Duty == nil || math.Abs(*m.Duty-50) > 2 {
		t.Errorf("duty = %v, want 50", m.Duty)
	}
}

func TestMeasureFlat(t *testing.T) {
	v := make([]float64, 300)
	raw := make([]int, 300)
	m := Measure(v, raw, 1e-6)
	if m.Freq != nil || m.Pp == nil || *m.Pp != 0 {
		t.Errorf("flat trace: freq %v pp %v", m.Freq, m.Pp)
	}
}

// pulse returns a 1 kHz pulse train over 6 ms in 600 samples: base −1 V,
// top 1 V, a linear rise of 4 samples (40 µs) and an overshoot of 0.2 V on
// the first sample after it, high for duty of the period.
func pulse(duty float64) ([]float64, []int) {
	const n = 600
	v := make([]float64, n)
	raw := make([]int, n)
	for i := range v {
		p := float64(i%100) + 0.5 // centred, so that edges fall between samples
		switch {
		case p < 4:
			v[i] = -1 + 2*p/4
		case p < 5:
			v[i] = 1.2
		case p < duty*100:
			v[i] = 1
		default:
			v[i] = -1
		}
		raw[i] = int(math.Round(v[i] * 25))
		v[i] = float64(raw[i]) / 25
	}
	return v, raw
}

func TestMeasurePulse(t *testing.T) {
	v, raw := pulse(0.3)
	m := Measure(v, raw, 6e-3/600)
	near := func(name string, got *float64, want, tol float64) {
		t.Helper()
		if got == nil || math.Abs(*got-want) > tol {
			t.Errorf("%s = %v, want %g", name, deref(got), want)
		}
	}
	near("top", m.Top, 1, 0.01)
	near("base", m.Base, -1, 0.01)
	near("amp", m.Amp, 2, 0.02)
	near("pp", m.Pp, 2.2, 0.02)
	near("overshoot", m.Overshoot, 10, 0.5)
	near("preshoot", m.Preshoot, 0, 0.5)
	// 10–90 % of a 4-sample linear rise is 3.2 samples.
	near("rise", m.Rise, 32e-6, 3e-6)
	near("pwidth", m.PWidth, 300e-6-20e-6, 12e-6)
	near("nwidth", m.NWidth, 700e-6+20e-6, 12e-6)
	near("freq", m.Freq, 1000, 1)
	near("duty", m.Duty, 28, 1.5)
	// A 0.3 duty pulse of ±1 V has a mean near −0.4 V over whole periods.
	near("cycAvg", m.CycAvg, -0.4, 0.05)
}

func TestMeasureSineLevels(t *testing.T) {
	// A sine has no flat top: top and base are the extremes.
	const n = 600
	v := make([]float64, n)
	raw := make([]int, n)
	for i := range v {
		raw[i] = int(math.Round(60 * math.Sin(2*math.Pi*5*float64(i)/n)))
		v[i] = float64(raw[i]) / 25
	}
	m := Measure(v, raw, 1e-6)
	if *m.Top != *m.Max || *m.Base != *m.Min {
		t.Errorf("top %g base %g, want max %g min %g", *m.Top, *m.Base, *m.Max, *m.Min)
	}
	// AC RMS of a 2.4 V amplitude sine.
	if math.Abs(*m.AcRms-2.4/math.Sqrt2) > 0.02 {
		t.Errorf("acRms = %g", *m.AcRms)
	}
}

func TestRelate(t *testing.T) {
	// CH2 is CH1 at half the amplitude, 45° later: an RC filter at its cutoff
	// would give 0.707 and the same lag.
	const n, cycles = 600, 5.0
	dt := 6e-3 / n
	mk := func(amp, lag float64) Measurements {
		v := make([]float64, n)
		raw := make([]int, n)
		for i := range v {
			raw[i] = int(math.Round(amp * 25 * math.Sin(2*math.Pi*cycles*float64(i)/n-lag)))
			v[i] = float64(raw[i]) / 25
		}
		return Measure(v, raw, dt)
	}
	a, b := mk(2, 0), mk(1, math.Pi/4)
	p := Relate(a, b, *a.Period, dt)
	period := 6e-3 / cycles
	if p.Phase == nil || math.Abs(*p.Phase+45) > 2 {
		t.Errorf("phase = %v, want −45", deref(p.Phase))
	}
	if p.Delay == nil || math.Abs(*p.Delay-period/8) > period/100 {
		t.Errorf("delay = %v, want %g", deref(p.Delay), period/8)
	}
	if p.Gain == nil || math.Abs(*p.Gain-0.5) > 0.02 {
		t.Errorf("gain = %v, want 0.5", deref(p.Gain))
	}
	// An offset and a noise spike move the 50 % level of CH2, but not the
	// phase of its fundamental.
	b = mk(1, math.Pi/4)
	for i := range b.v {
		b.v[i] += 0.3
	}
	b.v[123] += 0.5
	if p = Relate(a, b, *a.Period, dt); p.Phase == nil || math.Abs(*p.Phase+45) > 1 {
		t.Errorf("phase with offset = %v, want −45", deref(p.Phase))
	}
	// Leading by 170° stays +170°, not −190°.
	b = mk(1, -170*math.Pi/180)
	if p = Relate(a, b, *a.Period, dt); p.Phase == nil || math.Abs(*p.Phase-170) > 3 {
		t.Errorf("phase = %v, want 170", deref(p.Phase))
	}
}

func TestHarmonics(t *testing.T) {
	const n = 600
	dt := 6e-3 / n
	f := 5.3 / (n * dt)
	mk := func(h2, h3 float64) Measurements {
		v := make([]float64, n)
		raw := make([]int, n)
		for i := range v {
			noise := math.Sin(float64(i)*12.9898) * 43758.5453
			noise = 2 * (noise - math.Floor(noise) - 0.5)
			x := 2 * math.Pi * f * dt * float64(i)
			raw[i] = int(math.Round(25*(3*math.Sin(x)+h2*math.Sin(2*x+1)+h3*math.Sin(3*x+2)) + noise))
			v[i] = float64(raw[i]) / 25
		}
		return Measure(v, raw, dt)
	}
	fund, harm, ok := Harmonics(mk(0.09, 0.03), f, dt)
	if !ok {
		t.Fatal("no harmonics")
	}
	if math.Abs(cmplx.Abs(fund)/3-1) > 0.01 {
		t.Errorf("fundamental = %g, want 3", cmplx.Abs(fund))
	}
	// 113 samples a period: every harmonic up to the tenth.
	if len(harm) != MaxHarmonic-1 {
		t.Fatalf("%d harmonics, want %d", len(harm), MaxHarmonic-1)
	}
	h2, h3 := cmplx.Abs(harm[0]), cmplx.Abs(harm[1])
	if math.Abs(h2-0.09) > 0.005 || math.Abs(h3-0.03) > 0.005 {
		t.Errorf("2nd and 3rd = %g %g, want 0.09 0.03", h2, h3)
	}
	// The phases at the first sample: sin(2x+1) is cos at 1 − π/2.
	if d := cmplx.Phase(harm[0]) - (1 - math.Pi/2); math.Abs(d) > 0.1 {
		t.Errorf("phase of the 2nd off by %g rad", d)
	}
	// A clean sine: what is left is the noise of the counts.
	_, harm, _ = Harmonics(mk(0, 0), f, dt)
	var sq float64
	for _, h := range harm {
		sq += real(h)*real(h) + imag(h)*imag(h)
	}
	if thd := math.Sqrt(sq) / 3; thd > 0.003 {
		t.Errorf("distortion of a clean sine = %.4f", thd)
	}
	// Ten samples a period leave the 2nd harmonic only.
	if _, harm, _ := Harmonics(mk(0, 0), 0.1/dt, dt); len(harm) != 1 {
		t.Errorf("%d harmonics at 10 samples a period, want 1", len(harm))
	}
}

func TestRespond(t *testing.T) {
	const n = 600
	dt := 6e-3 / n
	// 5.3 periods, so that the screen does not end on a whole period.
	f := 5.3 / (n * dt)
	mk := func(amp, lag float64) Measurements {
		v := make([]float64, n)
		raw := make([]int, n)
		for i := range v {
			// Fixed pseudo-noise of up to a count either way.
			noise := math.Sin(float64(i)*12.9898) * 43758.5453
			noise = 2 * (noise - math.Floor(noise) - 0.5)
			raw[i] = int(math.Round(amp*25*math.Sin(2*math.Pi*f*dt*float64(i)-lag) + noise))
			v[i] = float64(raw[i]) / 25
		}
		return Measure(v, raw, dt)
	}
	a := mk(3, 0)
	// About two counts of signal in a count of noise: no edges of its own.
	b := mk(0.08, 80*math.Pi/180)
	r, ok := Respond(a, b, f, dt)
	if !ok {
		t.Fatal("no response")
	}
	if math.Abs(r.Gain/(0.08/3)-1) > 0.15 {
		t.Errorf("gain = %g, want %g", r.Gain, 0.08/3)
	}
	if math.Abs(r.Phase+80) > 6 {
		t.Errorf("phase = %g, want −80", r.Phase)
	}
	if math.Abs(r.Freq/f-1) > 0.01 {
		t.Errorf("freq = %g, want %g", r.Freq, f)
	}
	// One and a half periods are not enough.
	if _, ok := Respond(a, b, 1.5/(n*dt), dt); ok {
		t.Error("response from 1.5 periods")
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
