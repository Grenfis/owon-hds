package hds

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Screen geometry of the HDS272S, checked on the device.
const (
	DivPixels  = 25 // sample counts per vertical division
	DivSamples = 50 // samples per horizontal division
	DivsX      = 12
	DivsY      = 8
)

// Head is the reply of :DATa:WAVe:SCReen:HEAD?. Keys and values are
// lower-cased before parsing, as OWON's software does.
type Head struct {
	Timebase struct {
		Scale   string `json:"scale"`
		HOffset Num    `json:"hoffset"` // sample counts, DivPixels per division
	} `json:"timebase"`
	Sample struct {
		FullScreen Num    `json:"fullscreen"`
		SlowMove   Num    `json:"slowmove"`
		DataLen    Num    `json:"datalen"`
		SampleRate string `json:"samplerate"`
		Type       string `json:"type"`
		DepMem     string `json:"depmem"`
	} `json:"sample"`
	Channel   []HeadChannel `json:"channel"`
	DataType  string        `json:"datatype"`
	RunStatus string        `json:"runstatus"`
	Trig      struct {
		Mode  string `json:"mode"`
		Type  string `json:"type"`
		Items struct {
			Channel  string `json:"channel"`
			Level    string `json:"level"`
			Edge     string `json:"edge"`
			Coupling string `json:"coupling"`
			Holdoff  string `json:"holdoff"`
			Sweep    string `json:"sweep"`
		} `json:"items"`
		Sweep string `json:"sweep"` // older firmware keeps it here, outside items
	} `json:"trig"`
}

// HOffsetDivs is the horizontal offset in divisions, as :HORizontal:OFFSet
// takes it. A positive offset moves the trigger point left of the centre.
func (h *Head) HOffsetDivs() float64 {
	return float64(h.Timebase.HOffset) / DivPixels
}

func (h *Head) Sweep() string {
	if h.Trig.Items.Sweep != "" {
		return h.Trig.Items.Sweep
	}
	return h.Trig.Sweep
}

type HeadChannel struct {
	Name      string `json:"name"`
	Display   string `json:"display"`
	Coupling  string `json:"coupling"`
	Probe     string `json:"probe"`
	Scale     string `json:"scale"`
	Offset    Num    `json:"offset"` // trace position, sample counts from screen centre
	Frequence Num    `json:"frequence"`
	Inverse   string `json:"inverse"`
}

// Num accepts a JSON number or a numeric string; anything else reads as 0.
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	f, err := strconv.ParseFloat(strings.Trim(string(b), `"`), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		f = 0
	}
	*n = Num(f)
	return nil
}

func ParseHead(b []byte) (*Head, error) {
	var h Head
	if err := json.Unmarshal([]byte(strings.ToLower(strings.TrimSpace(string(b)))), &h); err != nil {
		return nil, err
	}
	// The sample rate keeps its case: lower-cased, "MSa/s" reads as milli.
	var orig struct{ Sample struct{ SampleRate string } }
	if json.Unmarshal(b, &orig) == nil && orig.Sample.SampleRate != "" {
		h.Sample.SampleRate = orig.Sample.SampleRate
	}
	return &h, nil
}

// VoltsPerDiv is the vertical scale the screen shows. The head reports it
// with the probe factor already applied: with a 10X probe and "1.00 V" a
// trace 40 counts high is 1.6 V, as the device measures it.
func (c *HeadChannel) VoltsPerDiv() float64 {
	return ParseUnit(c.Scale)
}

func (c *HeadChannel) On() bool { return c.Display == "on" }

// DecodeSamples turns a :DATa:WAVe:SCReen:CH<n>? payload into screen
// positions in sample counts from the centre. Each byte is one signed
// sample, 600 per screen. OWON's software reads every other byte only.
func DecodeSamples(p []byte) []int {
	out := make([]int, len(p))
	for i, b := range p {
		out[i] = int(int8(b))
	}
	return out
}

// ProbeFactor parses "10x" to 10.
func ProbeFactor(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "x"), 64)
	if err != nil || f <= 0 {
		return 1
	}
	return f
}

var unitPrefixes = []struct {
	suffix string
	mult   float64
}{
	{"ns", 1e-9}, {"us", 1e-6}, {"ms", 1e-3}, {"ks", 1e3}, {"s", 1},
	{"pv", 1e-12}, {"nv", 1e-9}, {"uv", 1e-6}, {"mv", 1e-3}, {"kv", 1e3}, {"v", 1},
	{"mhz", 1e6}, {"khz", 1e3}, {"hz", 1},
}

// ParseUnit reads values like "1.0ms", "200mv", "1.00 v", "0.00pv" or
// "1.000000e+04" into base units. "mhz" is read as megahertz, as the device
// prints it.
func ParseUnit(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "()")
	s = strings.TrimSuffix(s, "/div")
	mult := 1.0
	for _, u := range unitPrefixes {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSuffix(s, u.suffix), u.mult
			break
		}
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f * mult
}
