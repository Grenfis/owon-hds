package hds

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Screen geometry of the HDS272S, from OWON's model table ("HDS272S_1").
const (
	DivPixels = 25 // sample counts per division, both axes
	DivsX     = 12
	DivsY     = 8
)

// Head is the reply of :DATa:WAVe:SCReen:HEAD?. Keys and values are
// lower-cased before parsing, as OWON's software does.
type Head struct {
	Timebase struct {
		Scale   string `json:"scale"`
		HOffset Num    `json:"hoffset"`
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
		} `json:"items"`
		Sweep string `json:"sweep"`
	} `json:"trig"`
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
	return &h, nil
}

// VoltsPerDiv is the vertical scale the screen shows. OWON's software
// multiplies the head scale by the probe factor, so the head reports it at 1X.
func (c *HeadChannel) VoltsPerDiv() float64 {
	return ParseUnit(c.Scale) * ProbeFactor(c.Probe)
}

func (c *HeadChannel) On() bool { return c.Display == "on" }

// DecodeSamples turns a :DATa:WAVe:SCReen:CH<n>? payload into screen
// positions in sample counts from the centre. Each point takes two bytes;
// OWON's software reads the first one as a signed byte and skips the second.
func DecodeSamples(p []byte) []int {
	out := make([]int, len(p)/2)
	for i := range out {
		out[i] = int(int8(p[2*i]))
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
	{"uv", 1e-6}, {"mv", 1e-3}, {"kv", 1e3}, {"v", 1},
	{"mhz", 1e6}, {"khz", 1e3}, {"hz", 1},
}

// ParseUnit reads values like "1.0ms", "200mv", "2.00v" or "1.000000e+04"
// into base units. "mhz" is read as megahertz, as the device prints it.
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
