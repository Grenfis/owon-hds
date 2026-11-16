// Command owon-probe sends queries to a connected HDS272S and prints the raw
// replies, to check the protocol against the real device. Commands without
// "?" change settings and are refused unless -w is given; factory commands
// are refused always.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"owon-hds/internal/app"
	"owon-hds/internal/hds"
	"owon-hds/internal/usb"
)

var defaults = []string{
	"*IDN?", ":MODel?", ":RUNning?", ":TRIGger:STATus?",
	":DATa:WAVe:SCReen:HEAD?", ":DATa:WAVe:SCReen:CH1?", ":DATa:WAVe:SCReen:CH2?",
	":CH1:DISPlay?", ":CH1:SCALe?", ":CH1:PROBe?", ":CH1:OFFSet?", ":CH1:COUPling?",
	":CH2:DISPlay?", ":CH2:SCALe?", ":CH2:PROBe?", ":CH2:OFFSet?", ":CH2:COUPling?",
	":HORizontal:SCALe?", ":HORizontal:OFFSet?", ":ACQuire:MODe?", ":ACQuire:DEPMem?",
	":TRIGger:SINGle:EDGe:SOURce?", ":TRIGger:SINGle:EDGe?", ":TRIGger:SINGle:EDGe:LEVel?",
	":TRIGger:SWEep?", ":TRIGger:COUPling?",
	":FUNCtion?", ":FUNCtion:FREQuency?", ":FUNCtion:AMPLitude?", ":FUNCtion:OFFSet?",
	":FUNCtion:DTYCycle?", ":FUNCtion:SYMMetry?", ":CHANnel?",
	":MEASurement:CH1:FREQuency?", ":MEASurement:CH1:PKPK?", ":MEASurement:CH1:MAX?",
	":DMM:CONFigure?", ":DMM:MEAS?",
}

func main() {
	write := flag.Bool("w", false, "allow setting commands")
	raw := flag.Bool("raw", false, "print every USB read instead of parsing replies")
	frames := flag.Int("frames", 0, "read this many frames as the application does")
	quiet := flag.Duration("quiet", 300*time.Millisecond, "raw mode: stop reading after this long without data")
	flag.Parse()
	cmds := flag.Args()
	if len(cmds) == 0 {
		cmds = defaults
	}
	d, err := usb.Open(0x5345, 0x1234)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	fmt.Printf("serial %q, %s\n\n", d.Serial, d.Summary)
	if *raw {
		rawDump(d, cmds, *write, *quiet)
		return
	}
	c := hds.New(d)
	defer c.Close()
	if *frames > 0 {
		readFrames(c, *frames)
		return
	}

	for _, cmd := range cmds {
		if app.Blocked(cmd) {
			fmt.Printf("%-34s refused: factory command\n", cmd)
			continue
		}
		start := time.Now()
		if !strings.HasSuffix(cmd, "?") {
			if !*write {
				fmt.Printf("%-34s refused: not a query, use -w\n", cmd)
				continue
			}
			if err := c.Send(cmd); err != nil {
				fmt.Printf("%-34s error: %v\n", cmd, err)
			} else {
				fmt.Printf("%-34s sent\n", cmd)
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if strings.HasPrefix(strings.ToUpper(cmd), ":DAT") {
			b, err := c.QueryData(cmd)
			ms := time.Since(start).Milliseconds()
			if err != nil {
				fmt.Printf("%-34s %4d ms  error: %v\n", cmd, ms, err)
				continue
			}
			fmt.Printf("%-34s %4d ms  %d bytes\n", cmd, ms, len(b))
			if printable(b) {
				fmt.Printf("    %s\n", b)
			} else {
				fmt.Print(indent(hex.Dump(b[:min(len(b), 96)])))
				fmt.Printf("    rising zero crossings of int8 bytes: %v\n", rising(b))
			}
			continue
		}
		s, err := c.Query(cmd)
		ms := time.Since(start).Milliseconds()
		if err != nil {
			fmt.Printf("%-34s %4d ms  error: %v\n", cmd, ms, err)
			continue
		}
		fmt.Printf("%-34s %4d ms  %q\n", cmd, ms, s)
	}
}

// readFrames reads the head and CH1 like the application's loop and prints
// the sample range of each frame.
func readFrames(c *hds.Client, n int) {
	for i := 0; i < n; i++ {
		start := time.Now()
		h, err := c.QueryData(":DATa:WAVe:SCReen:HEAD?")
		if err != nil {
			fmt.Printf("frame %d head: %v\n", i, err)
			continue
		}
		p, err := c.QueryData(":DATa:WAVe:SCReen:CH1?")
		if err != nil {
			fmt.Printf("frame %d ch1: %v\n", i, err)
			continue
		}
		s := hds.DecodeSamples(p)
		lo, hi := s[0], s[0]
		for _, v := range s {
			lo, hi = min(lo, v), max(hi, v)
		}
		fmt.Printf("frame %d  %3d ms  head %d bytes  ch1 %d samples  min %d max %d  first % x\n",
			i, time.Since(start).Milliseconds(), len(h), len(s), lo, hi, p[:12])
	}
}

// rawDump sends each command as the client would and prints every read
// until the device stays quiet for quiet. Setting commands need -w and are
// followed by the next command without waiting.
func rawDump(d *usb.Device, cmds []string, write bool, quiet time.Duration) {
	defer d.Close()
	buf := make([]byte, 16*1024)
	for _, cmd := range cmds {
		if app.Blocked(cmd) || !strings.HasSuffix(cmd, "?") && !write {
			fmt.Printf("%-34s refused\n", cmd)
			continue
		}
		if !strings.HasSuffix(cmd, "?") {
			if _, err := d.Write([]byte(cmd)); err != nil {
				fmt.Printf("%-34s write: %v\n", cmd, err)
			} else {
				fmt.Printf("%-34s sent\n", cmd)
			}
			continue
		}
		w := cmd
		if strings.HasPrefix(strings.ToUpper(cmd), ":DAT") {
			w += "\r\n"
		}
		start := time.Now()
		if _, err := d.Write([]byte(w)); err != nil {
			fmt.Printf("%-34s write: %v\n", cmd, err)
			continue
		}
		fmt.Println(cmd)
		for {
			n, err := d.Read(buf, quiet)
			if err != nil {
				fmt.Printf("    %4d ms  %v\n", time.Since(start).Milliseconds(), err)
				break
			}
			fmt.Printf("    %4d ms  %4d bytes  % x\n", time.Since(start).Milliseconds(), n, buf[:min(n, 24)])
		}
	}
}

// rising lists the byte indices where the signal, read as int8 samples,
// crosses zero upwards.
func rising(b []byte) []int {
	var out []int
	for i := 1; i < len(b); i++ {
		if int8(b[i-1]) < 0 && int8(b[i]) >= 0 {
			out = append(out, i)
		}
	}
	return out
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' || c > 0x7e {
			return false
		}
	}
	return true
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n    ") + "\n"
}
