package hds

import (
	"encoding/binary"
	"testing"
	"time"
)

// fakeTransport returns queued chunks; Read never waits.
type fakeTransport struct {
	queue  [][]byte
	writes []string
	reply  func(cmd string) []byte
}

func (f *fakeTransport) Write(p []byte) (int, error) {
	f.writes = append(f.writes, string(p))
	if r := f.reply(string(p)); r != nil {
		f.queue = append(f.queue, r)
	}
	return len(p), nil
}

func (f *fakeTransport) Read(p []byte, _ time.Duration) (int, error) {
	if len(f.queue) == 0 {
		return 0, ErrTimeout
	}
	n := copy(p, f.queue[0])
	f.queue = f.queue[1:]
	return n, nil
}

func (f *fakeTransport) Close() error { return nil }

func dataReply(payload string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))
	return append(b, payload...)
}

func TestLateReplyIsDiscarded(t *testing.T) {
	late := true
	f := &fakeTransport{}
	f.reply = func(cmd string) []byte {
		switch cmd {
		case ":DATa:WAVe:SCReen:HEAD?\r\n":
			if late {
				return nil // the device is busy and answers after the timeout
			}
			return dataReply(`{"head":1}`)
		case ":DATa:WAVe:SCReen:CH1?\r\n":
			return dataReply("samples")
		}
		return nil
	}
	c := New(f)
	if _, err := c.QueryData(":DATa:WAVe:SCReen:HEAD?"); err == nil {
		t.Fatal("expected a timeout")
	}
	f.queue = append(f.queue, dataReply(`{"head":1}`)) // the late reply
	late = false
	p, err := c.QueryData(":DATa:WAVe:SCReen:CH1?")
	if err != nil || string(p) != "samples" {
		t.Fatalf("CH1 = %q, %v; want the CH1 reply, not the late head", p, err)
	}
}
