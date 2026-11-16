// Package app is the application core shared by the desktop window and the
// browser mode: it owns the connection, runs the acquisition loop and exposes
// the methods the UI calls.
package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"owon-hds/internal/hds"
	"owon-hds/internal/sim"
	"owon-hds/internal/usb"
)

// Emitter delivers an event to the UI.
type Emitter func(event string, data any)

type Status struct {
	Connected    bool   `json:"connected"`
	Mode         string `json:"mode"` // "usb" or "sim"
	IDN          string `json:"idn"`
	Detail       string `json:"detail"`
	Error        string `json:"error"`
	Reconnecting bool   `json:"reconnecting"`
}

type App struct {
	emit Emitter
	// SaveDialog asks where to save a file; nil when the shell has no dialogs.
	SaveDialog func(defaultName string) (string, error)

	mu     sync.Mutex
	status Status
	client *hds.Client
	cancel context.CancelFunc
	done   chan struct{}

	queue   *queue
	view    atomic.Value // "scope", "dmm" or "console"
	acquire atomic.Bool
	seq     int64
}

func New(emit Emitter) *App {
	a := &App{emit: emit, queue: newQueue()}
	a.view.Store("scope")
	a.acquire.Store(true)
	return a
}

// Shutdown closes the connection; the shell calls it on exit.
func (a *App) Shutdown() { a.Disconnect() }

// Connect opens the device ("usb") or the simulator ("sim").
func (a *App) Connect(mode string) (Status, error) {
	a.Disconnect()
	a.queue.take() // settings queued while offline would hit the device out of context
	c, st, err := open(mode)
	if err != nil {
		a.setStatus(Status{Mode: mode, Error: err.Error()})
		return a.GetStatus(), err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel, a.done = cancel, make(chan struct{})
	done := a.done
	a.mu.Unlock()
	go func() {
		defer close(done)
		a.supervise(ctx, mode, c, st)
	}()
	return st, nil
}

func (a *App) Disconnect() {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.cancel, a.done = nil, nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	a.setStatus(Status{})
}

func (a *App) GetStatus() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func open(mode string) (*hds.Client, Status, error) {
	st := Status{Mode: mode}
	var t hds.Transport
	switch mode {
	case "sim":
		t = sim.New()
		st.Detail = "Имитатор: CH1 — выход генератора, CH2 — тот же сигнал после RC-фильтра 10 кГц"
	case "usb":
		d, err := usb.Open(0x5345, 0x1234)
		if err != nil {
			return nil, st, err
		}
		t = d
		st.Detail = d.Summary
	default:
		return nil, st, fmt.Errorf("неизвестный режим %q", mode)
	}
	c := hds.New(t)
	idn, err := c.Query("*IDN?")
	if err != nil {
		c.Close()
		return nil, st, fmt.Errorf("прибор не ответил на *IDN?: %w", err)
	}
	st.Connected, st.IDN = true, idn
	return c, st, nil
}

// supervise runs the loop and reconnects after the device drops out, for
// example when the cable is pulled or the device goes to sleep.
func (a *App) supervise(ctx context.Context, mode string, c *hds.Client, st Status) {
	for {
		a.mu.Lock()
		a.client = c
		a.mu.Unlock()
		a.setStatus(st)
		err := a.loop(ctx, c)
		a.mu.Lock()
		a.client = nil
		a.mu.Unlock()
		c.Close()
		if ctx.Err() != nil {
			return
		}
		lost := Status{Mode: mode, Error: "связь потеряна: " + err.Error(), Reconnecting: true}
		for {
			a.setStatus(lost)
			if !sleep(ctx, 2*time.Second) {
				return
			}
			if c, st, err = open(mode); err == nil {
				break
			}
			lost.Error = "переподключение: " + err.Error()
		}
	}
}

func (a *App) loop(ctx context.Context, c *hds.Client) error {
	var last time.Time
	var fps float64
	misses := 0
	// The first frame after a scale change, autoset or screen switch may
	// pair the new settings with a trace taken before, so it is not shown.
	skip := false
	// The device screen, read once, so that a view that already matches it
	// does not switch it again.
	screen, _ := c.Query(":MODE?")
	for ctx.Err() == nil {
		if want := viewScreens[a.view.Load().(string)]; want != "" && !strings.EqualFold(want, screen) {
			cmd := ":MODE " + want
			if err := c.Send(cmd); err != nil {
				return err
			}
			a.emit("log", LogEntry{Dir: "out", Text: cmd, Time: time.Now().UnixMilli()})
			screen, skip = want, true
		}
		for _, cmd := range a.queue.take() {
			if err := c.Send(cmd); err != nil {
				return err
			}
			skip = skip || scaleCommand.MatchString(cmd)
			a.emit("log", LogEntry{Dir: "out", Text: cmd, Time: time.Now().UnixMilli()})
		}
		switch a.view.Load() {
		case "scope":
			if !a.acquire.Load() {
				a.wait(ctx, 200*time.Millisecond)
				continue
			}
			f, err := a.frame(c)
			if err != nil {
				// A single lost frame happens when a setting changes mid-read.
				if misses++; misses >= 3 {
					return err
				}
				continue
			}
			misses = 0
			if skip {
				skip = false
				continue
			}
			now := time.Now()
			if !last.IsZero() {
				inst := 1 / now.Sub(last).Seconds()
				if fps == 0 {
					fps = inst
				}
				fps += (inst - fps) * 0.2
			}
			last = now
			f.FPS = fps
			a.emit("frame", f)
		case "dmm":
			s, err := c.Query(":DMM:MEAS?")
			if err != nil {
				if misses++; misses >= 3 {
					return err
				}
				continue
			}
			misses = 0
			a.emit("dmm", DMMReading{Raw: s, Time: time.Now().UnixMilli()})
			a.wait(ctx, 250*time.Millisecond)
		default:
			a.wait(ctx, 200*time.Millisecond)
		}
	}
	return ctx.Err()
}

var scaleCommand = regexp.MustCompile(`^:(CH[12]:SCAL|AUToseton)`)

// viewScreens is the device screen each view needs: :MODE switches it, and
// :MODE? reports it. The console leaves the screen as it is.
var viewScreens = map[string]string{"scope": "OSC", "dmm": "DMM"}

// wait sleeps until d passes, a setting is queued or the context ends.
func (a *App) wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-a.queue.wake:
	case <-time.After(d):
	}
}

type FrameTrig struct {
	Source   string  `json:"source"`
	Edge     string  `json:"edge"`
	Sweep    string  `json:"sweep"`
	Coupling string  `json:"coupling"`
	Level    float64 `json:"level"`
	LevelStr string  `json:"levelStr"`
}

type FrameChannel struct {
	Name        string           `json:"name"`
	On          bool             `json:"on"`
	Coupling    string           `json:"coupling"`
	Probe       string           `json:"probe"`
	VoltsPerDiv float64          `json:"voltsPerDiv"`
	Offset      int              `json:"offset"` // sample counts from screen centre
	Raw         []int            `json:"raw"`
	Meas        hds.Measurements `json:"meas"`
}

type Frame struct {
	Seq        int64          `json:"seq"`
	Run        string         `json:"run"`
	TimeDiv    float64        `json:"timeDiv"`
	TimeScale  string         `json:"timeScale"`
	HOffset    float64        `json:"hOffset"`
	SampleRate string         `json:"sampleRate"`
	AcqMode    string         `json:"acqMode"`
	DepMem     string         `json:"depMem"`
	Trig       FrameTrig      `json:"trig"`
	Channels   []FrameChannel `json:"channels"`
	// Pair relates CH2 to CH1 when both are on.
	Pair *hds.Pair `json:"pair"`
	FPS  float64   `json:"fps"`
}

func (a *App) frame(c *hds.Client) (*Frame, error) {
	b, err := c.QueryData(":DATa:WAVe:SCReen:HEAD?")
	if err != nil {
		return nil, err
	}
	h, err := hds.ParseHead(b)
	if err != nil {
		c.Resync()
		return nil, fmt.Errorf("заголовок осциллограммы: %w", err)
	}
	a.seq++
	f := &Frame{
		Seq: a.seq, Run: h.RunStatus,
		TimeDiv: hds.ParseUnit(h.Timebase.Scale), TimeScale: h.Timebase.Scale, HOffset: h.HOffsetDivs(),
		SampleRate: strings.Trim(h.Sample.SampleRate, "()"), AcqMode: h.Sample.Type, DepMem: h.Sample.DepMem,
		Trig: FrameTrig{
			Source: h.Trig.Items.Channel, Edge: h.Trig.Items.Edge, Sweep: h.Sweep(),
			Coupling: h.Trig.Items.Coupling, LevelStr: h.Trig.Items.Level, Level: hds.ParseUnit(h.Trig.Items.Level),
		},
	}
	for _, hc := range h.Channel {
		ch := FrameChannel{
			Name: strings.ToUpper(hc.Name), On: hc.On(), Coupling: hc.Coupling, Probe: hc.Probe,
			VoltsPerDiv: hc.VoltsPerDiv(), Offset: int(hc.Offset),
		}
		if ch.On {
			p, err := c.QueryData(":DATa:WAVe:SCReen:" + ch.Name + "?")
			if err != nil {
				return nil, err
			}
			// A reply of the wrong length belongs to another query.
			if want := int(h.Sample.DataLen); want > 0 && len(p) != want {
				c.Resync()
				return nil, fmt.Errorf("%s: %d байт вместо %d", ch.Name, len(p), want)
			}
			ch.Raw = hds.DecodeSamples(p)
			if n := len(ch.Raw); n > 0 {
				volts := make([]float64, n)
				for i, r := range ch.Raw {
					volts[i] = float64(r-ch.Offset) / hds.DivPixels * ch.VoltsPerDiv
				}
				ch.Meas = hds.Measure(volts, ch.Raw, f.TimeDiv*hds.DivsX/float64(n))
			}
			// The device counts the frequency itself, and more precisely
			// than a few periods on the screen allow.
			if fr := float64(hc.Frequence); fr > 0 {
				period := 1 / fr
				ch.Meas.Freq, ch.Meas.Period = &fr, &period
			}
		}
		f.Channels = append(f.Channels, ch)
	}
	if len(f.Channels) == 2 && f.Channels[0].On && f.Channels[1].On && len(f.Channels[0].Raw) > 0 {
		a, b := f.Channels[0].Meas, f.Channels[1].Meas
		var period float64
		if a.Period != nil {
			period = *a.Period
		}
		p := hds.Relate(a, b, period, f.TimeDiv*hds.DivsX/float64(len(f.Channels[0].Raw)))
		f.Pair = &p
	}
	return f, nil
}

// Set queues a setting change. Repeated changes of one setting before the
// loop sends them collapse into the last value, so turning a knob fast does
// not build up a backlog.
func (a *App) Set(key, value string) error {
	cmd, err := command(key, value)
	if err != nil {
		return err
	}
	a.queue.put(key, cmd)
	return nil
}

// SetView tells the loop what to poll: "scope", "dmm" or "console".
func (a *App) SetView(view string) {
	a.view.Store(view)
	a.queue.poke()
}

// SetAcquire pauses or resumes waveform polling without touching the device.
func (a *App) SetAcquire(on bool) {
	a.acquire.Store(on)
	a.queue.poke()
}

type GenState struct {
	Wave     string   `json:"wave"`
	Freq     *float64 `json:"freq"`
	Amp      *float64 `json:"amp"`
	Offset   *float64 `json:"offset"`
	Duty     *float64 `json:"duty"`
	Symmetry *float64 `json:"symmetry"`
	Output   *bool    `json:"output"`
}

// ReadGen reads the generator settings. Values the device did not report
// stay nil.
func (a *App) ReadGen() (GenState, error) {
	c := a.currentClient()
	if c == nil {
		return GenState{}, errors.New("нет связи с прибором")
	}
	var g GenState
	num := func(cmd string) *float64 {
		s, err := c.Query(cmd)
		if err != nil {
			return nil
		}
		f, err := strconv.ParseFloat(strings.TrimRight(strings.TrimSpace(s), "%"), 64)
		if err != nil {
			return nil
		}
		return &f
	}
	if s, err := c.Query(":FUNCtion?"); err == nil {
		g.Wave = strings.ToUpper(s)
	}
	g.Freq = num(":FUNCtion:FREQuency?")
	g.Amp = num(":FUNCtion:AMPLitude?")
	g.Offset = num(":FUNCtion:OFFSet?")
	g.Duty = num(":FUNCtion:DTYCycle?")
	g.Symmetry = num(":FUNCtion:SYMMetry?")
	if s, err := c.Query(":CHANnel?"); err == nil {
		on := strings.EqualFold(s, "ON") || s == "1"
		g.Output = &on
	}
	return g, nil
}

// DeviceState holds device settings outside the scope and the generator,
// in the values the "dev." settings take: "30" or "UNLIMITED".
type DeviceState struct {
	Shutdown  string `json:"shutdown"`  // minutes without key presses before power-off
	Backlight string `json:"backlight"` // seconds without key presses before the screen goes dark
}

// ReadDevice reads the auto-shutdown and backlight times. A value the device
// did not report stays empty.
func (a *App) ReadDevice() (DeviceState, error) {
	c := a.currentClient()
	if c == nil {
		return DeviceState{}, errors.New("нет связи с прибором")
	}
	read := func(cmd, unit string) string {
		s, err := c.Query(cmd)
		if err != nil {
			return ""
		}
		return strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(s), unit))
	}
	return DeviceState{
		Shutdown:  read(":SHUTdown:TIMe?", "min"),
		Backlight: read(":DISPlay:BACKlighttime?", "s"),
	}, nil
}

type LogEntry struct {
	Dir  string `json:"dir"` // "out", "in" or "err"
	Text string `json:"text"`
	Time int64  `json:"time"`
}

type ConsoleResult struct {
	Reply string `json:"reply"`
	Kind  string `json:"kind"` // "text", "data" or "none"
	Bytes int    `json:"bytes"`
	Ms    int64  `json:"ms"`
}

// Console sends a raw SCPI command typed by the user.
func (a *App) Console(cmd string) (ConsoleResult, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ConsoleResult{}, errors.New("пустая команда")
	}
	if Blocked(cmd) {
		return ConsoleResult{}, errors.New("заводская или калибровочная команда: она пишет в память прибора, поэтому заблокирована")
	}
	c := a.currentClient()
	if c == nil {
		return ConsoleResult{}, errors.New("нет связи с прибором")
	}
	start := time.Now()
	header, _, _ := strings.Cut(cmd, " ")
	var r ConsoleResult
	switch {
	case strings.HasPrefix(strings.ToUpper(strings.TrimLeft(header, ":")), "DAT") && strings.HasSuffix(header, "?"):
		p, err := c.QueryData(cmd)
		if err != nil {
			return r, err
		}
		r.Kind, r.Bytes = "data", len(p)
		if json.Valid(p) {
			r.Reply = string(p)
		} else {
			r.Reply = hexPreview(p, 64)
		}
	case strings.HasSuffix(header, "?"):
		s, err := c.Query(cmd)
		if err != nil {
			return r, err
		}
		r.Kind, r.Reply, r.Bytes = "text", s, len(s)
	default:
		if err := c.Send(cmd); err != nil {
			return r, err
		}
		r.Kind = "none"
	}
	r.Ms = time.Since(start).Milliseconds()
	return r, nil
}

type DMMReading struct {
	Raw  string `json:"raw"`
	Time int64  `json:"time"`
}

// SaveFile asks where to save and writes base64-encoded content there.
// It returns the chosen path, or "" when the user cancelled.
func (a *App) SaveFile(defaultName, b64 string) (string, error) {
	if a.SaveDialog == nil {
		return "", errors.New("сохранение недоступно в этом режиме")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	path, err := a.SaveDialog(defaultName)
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

func (a *App) currentClient() *hds.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

func (a *App) setStatus(s Status) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.emit("status", s)
}

func hexPreview(p []byte, n int) string {
	var b strings.Builder
	for i, x := range p[:min(n, len(p))] {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02x", x)
	}
	if len(p) > n {
		b.WriteString(" …")
	}
	return b.String()
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// queue holds the latest command per setting, in first-change order.
type queue struct {
	mu    sync.Mutex
	order []string
	cmds  map[string]string
	wake  chan struct{}
}

func newQueue() *queue {
	return &queue{cmds: map[string]string{}, wake: make(chan struct{}, 1)}
}

func (q *queue) put(key, cmd string) {
	q.mu.Lock()
	if _, ok := q.cmds[key]; !ok {
		q.order = append(q.order, key)
	}
	q.cmds[key] = cmd
	q.mu.Unlock()
	q.poke()
}

func (q *queue) poke() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *queue) take() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]string, 0, len(q.order))
	for _, k := range q.order {
		out = append(out, q.cmds[k])
	}
	q.order, q.cmds = q.order[:0], map[string]string{}
	return out
}
