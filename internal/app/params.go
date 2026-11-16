package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type kind int

const (
	kindEnum   kind = iota
	kindNumber      // plain number, sent without unit
	kindUnit        // number with an optional unit suffix, as "200us" or "1.00V"
	kindNone        // command without an argument
)

// param is a setting the UI may change. Only these are sent from the UI
// controls; factory and calibration commands are not reachable from here.
type param struct {
	cmd      string
	kind     kind
	enum     []string
	min, max float64
}

var unitValue = regexp.MustCompile(`(?i)^-?[0-9]+(\.[0-9]+)?(ns|us|ms|ks|s|uv|mv|kv|v)?$`)

var params = func() map[string]param {
	onOff := []string{"ON", "OFF"}
	m := map[string]param{
		"hor.scale":     {cmd: ":HORizontal:SCALe %s", kind: kindUnit},
		"hor.offset":    {cmd: ":HORizontal:OFFSet %s", kind: kindNumber, min: -10000, max: 10000},
		"acq.mode":      {cmd: ":ACQuire:MODe %s", kind: kindEnum, enum: []string{"SAMPle", "PEAK"}},
		"acq.depmem":    {cmd: ":ACQuire:DEPMem %s", kind: kindEnum, enum: []string{"4K", "8K"}},
		"trig.source":   {cmd: ":TRIGger:SINGle:EDGe:SOURce %s", kind: kindEnum, enum: []string{"CH1", "CH2"}},
		"trig.edge":     {cmd: ":TRIGger:SINGle:EDGe %s", kind: kindEnum, enum: []string{"RISE", "FALL"}},
		"trig.level":    {cmd: ":TRIGger:SINGle:EDGe:LEVel %s", kind: kindUnit},
		"trig.sweep":    {cmd: ":TRIGger:SWEep %s", kind: kindEnum, enum: []string{"AUTO", "NORMal", "SINGle"}},
		"trig.coupling": {cmd: ":TRIGger:COUPling %s", kind: kindEnum, enum: []string{"DC", "AC"}},
		"run":           {cmd: ":RUNning %s", kind: kindEnum, enum: []string{"RUN", "STOP"}},
		// The device runs a set handler only when the command has an
		// argument; the autoset handler ignores it.
		"auto":         {cmd: ":AUToseton ON", kind: kindNone},
		"meas.display": {cmd: ":MEASurement:DISPlay %s", kind: kindEnum, enum: onOff},

		"gen.wave": {cmd: ":FUNCtion %s", kind: kindEnum, enum: []string{"SINE", "SQUare", "RAMP", "PULSe",
			"AmpALT", "AttALT", "StairDn", "StairUD", "StairUp", "Besselj", "Bessely", "Sinc"}},
		"gen.freq":     {cmd: ":FUNCtion:FREQuency %s", kind: kindNumber, min: 0.1, max: 25e6},
		"gen.period":   {cmd: ":FUNCtion:PERiod %s", kind: kindNumber, min: 4e-8, max: 10},
		"gen.amp":      {cmd: ":FUNCtion:AMPLitude %s", kind: kindNumber, min: 0.01, max: 5},
		"gen.offset":   {cmd: ":FUNCtion:OFFSet %s", kind: kindNumber, min: -2.5, max: 2.5},
		"gen.high":     {cmd: ":FUNCtion:HIGHt %s", kind: kindNumber, min: -2.5, max: 2.5},
		"gen.low":      {cmd: ":FUNCtion:LOW %s", kind: kindNumber, min: -2.5, max: 2.5},
		"gen.duty":     {cmd: ":FUNCtion:DTYCycle %s", kind: kindNumber, min: 0, max: 100},
		"gen.symmetry": {cmd: ":FUNCtion:SYMMetry %s", kind: kindNumber, min: 0, max: 100},
		"gen.width":    {cmd: ":FUNCtion:WIDTh %s", kind: kindNumber, min: 0, max: 10},
		"gen.rise":     {cmd: ":FUNCtion:RISing %s", kind: kindNumber, min: 0, max: 10},
		"gen.fall":     {cmd: ":FUNCtion:FALing %s", kind: kindNumber, min: 0, max: 10},
		"gen.output":   {cmd: ":CHANnel %s", kind: kindEnum, enum: onOff},

		"dmm.voltage": {cmd: ":DMM:CONFigure:VOLTage %s", kind: kindEnum, enum: []string{"DC", "AC"}},
		"dmm.current": {cmd: ":DMM:CONFigure:CURRent %s", kind: kindEnum, enum: []string{"DC", "AC"}},
		"dmm.func": {cmd: ":DMM:CONFigure %s", kind: kindEnum,
			enum: []string{"RESistance", "DIODe", "CONTinuity", "CAPacitance"}},
		"dmm.rel":   {cmd: ":DMM:REL %s", kind: kindEnum, enum: onOff},
		"dmm.auto":  {cmd: ":DMM:AUTo ON", kind: kindNone},
		"dmm.range": {cmd: ":DMM:RANGe ON", kind: kindNone},
	}
	for _, ch := range []string{"CH1", "CH2"} {
		k := strings.ToLower(ch) + "."
		m[k+"display"] = param{cmd: ":" + ch + ":DISPlay %s", kind: kindEnum, enum: onOff}
		m[k+"coupling"] = param{cmd: ":" + ch + ":COUPling %s", kind: kindEnum, enum: []string{"DC", "AC", "GND"}}
		m[k+"probe"] = param{cmd: ":" + ch + ":PROBe %s", kind: kindEnum, enum: []string{"1X", "10X", "100X", "1000X"}}
		m[k+"scale"] = param{cmd: ":" + ch + ":SCALe %s", kind: kindUnit}
		m[k+"offset"] = param{cmd: ":" + ch + ":OFFSet %s", kind: kindNumber, min: -8, max: 8}
	}
	return m
}()

// command builds the SCPI command for a UI setting.
func command(key, value string) (string, error) {
	p, ok := params[key]
	if !ok {
		return "", fmt.Errorf("неизвестный параметр %q", key)
	}
	value = strings.TrimSpace(value)
	switch p.kind {
	case kindNone:
		return p.cmd, nil
	case kindEnum:
		for _, e := range p.enum {
			if strings.EqualFold(e, value) {
				return fmt.Sprintf(p.cmd, e), nil
			}
		}
		return "", fmt.Errorf("%s: недопустимое значение %q", key, value)
	case kindUnit:
		if !unitValue.MatchString(value) {
			return "", fmt.Errorf("%s: недопустимое значение %q", key, value)
		}
		return fmt.Sprintf(p.cmd, value), nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || f < p.min || f > p.max {
		return "", fmt.Errorf("%s: значение %q вне диапазона %g…%g", key, value, p.min, p.max)
	}
	return fmt.Sprintf(p.cmd, formatNumber(f)), nil
}

// formatNumber prints a plain decimal without exponent and trailing zeros.
func formatNumber(f float64) string {
	s := strconv.FormatFloat(f, 'f', 9, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// blockedRoots are factory, calibration and firmware-upload commands. They
// write to the device memory without confirmation, so the console refuses
// them in both long and short form. "AUT" is the short form of both
// :AUToseton and :AUTocalibration, so only the long :AUToseton is allowed.
var blockedRoots = map[string]bool{
	"SERIAL": true, "SER": true,
	"SELFCORREC": true, "S": true,
	"AUTOCALIBRATION": true, "AUT": true,
	"PRACTISE": true,
	"DOWNLOAD": true, "DOWN": true,
	"VIDEO":   true,
	"VOLFULL": true,
}

// Blocked reports whether cmd starts with one of blockedRoots.
func Blocked(cmd string) bool {
	header, _, _ := strings.Cut(strings.TrimSpace(cmd), " ")
	root, _, _ := strings.Cut(strings.Trim(strings.ToUpper(header), ":*?"), ":")
	return blockedRoots[strings.TrimSuffix(root, "?")]
}
