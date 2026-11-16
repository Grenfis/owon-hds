package hds

import (
	"math"
	"testing"
)

func TestParseUnit(t *testing.T) {
	for in, want := range map[string]float64{
		"1.0ms": 1e-3, "200us": 200e-6, "2.0ns": 2e-9, "1000s": 1000,
		"10.0mv": 0.01, "1.00v": 1, "2.00kV": 2000, "500uv": 500e-6,
		"1.000000e+04": 1e4, "(500ks/s)": 0, "25mv": 0.025, "-1.5v": -1.5,
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
	if v := h.Channel[0].VoltsPerDiv(); math.Abs(v-0.05) > 1e-12 {
		t.Errorf("VoltsPerDiv = %g, want 0.05", v)
	}
}

func TestDecodeSamples(t *testing.T) {
	got := DecodeSamples([]byte{0x05, 0x00, 0xfb, 0xff, 0x7f, 0x00, 0x80, 0xff})
	want := []int{5, -5, 127, -128}
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

func TestMeasureFlat(t *testing.T) {
	v := make([]float64, 300)
	raw := make([]int, 300)
	m := Measure(v, raw, 1e-6)
	if m.Freq != nil || m.Pp == nil || *m.Pp != 0 {
		t.Errorf("flat trace: freq %v pp %v", m.Freq, m.Pp)
	}
}
