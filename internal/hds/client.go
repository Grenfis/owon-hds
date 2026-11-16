// Package hds talks to an OWON HDS200-series handheld oscilloscope (HDS272S)
// over its USB SCPI interface.
//
// Wire format, taken from OWON's own PC software and firmware V8.8.2:
//   - a command is a plain ASCII string, the :DATa queries end with "\r\n";
//   - a set command has no reply;
//   - a text query replies once, sometimes with a trailing "->" prompt;
//   - a :DATa query replies with a 4-byte little-endian length and the payload.
package hds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Transport is a raw bidirectional byte pipe to the device.
type Transport interface {
	Write(p []byte) (int, error)
	// Read returns what the device sent within timeout. A timeout without
	// data returns ErrTimeout.
	Read(p []byte, timeout time.Duration) (int, error)
	Close() error
}

// ErrTimeout is returned by Transport.Read when no data arrived in time.
var ErrTimeout = errors.New("нет ответа от прибора")

const (
	ioTimeout   = time.Second
	readBufSize = 16 * 1024
	maxDataSize = 4 << 20
)

// Client serialises SCPI transactions over one Transport.
type Client struct {
	mu  sync.Mutex
	t   Transport
	buf []byte
	// stale is set after a command without reply: if the device answered it
	// anyway, the next query would read that answer instead of its own.
	stale bool
}

func New(t Transport) *Client {
	return &Client{t: t, buf: make([]byte, readBufSize)}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t.Close()
}

// Send writes a command that has no reply.
func (c *Client) Send(cmd string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stale = true
	_, err := c.t.Write([]byte(cmd))
	return err
}

// Query sends a command and returns its text reply.
func (c *Client) Query(cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drain()
	if _, err := c.t.Write([]byte(cmd)); err != nil {
		return "", err
	}
	n, err := c.t.Read(c.buf, ioTimeout)
	if err != nil {
		return "", err
	}
	return CleanReply(c.buf[:n]), nil
}

// QueryData sends a :DATa query and returns the payload without its length.
func (c *Client) QueryData(cmd string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drain()
	if _, err := c.t.Write([]byte(cmd + "\r\n")); err != nil {
		return nil, err
	}
	n, err := c.t.Read(c.buf, ioTimeout)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		return nil, fmt.Errorf("%s: короткий ответ (%d байт)", cmd, n)
	}
	size := int(binary.LittleEndian.Uint32(c.buf))
	if size > maxDataSize {
		c.stale = true
		return nil, fmt.Errorf("%s: неверная длина ответа %d", cmd, size)
	}
	out := make([]byte, 0, size)
	out = append(out, c.buf[4:n]...)
	for misses := 0; len(out) < size; {
		n, err = c.t.Read(c.buf, ioTimeout)
		if errors.Is(err, ErrTimeout) {
			if misses++; misses > 2 {
				return nil, fmt.Errorf("%s: получено %d из %d байт", cmd, len(out), size)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, c.buf[:n]...)
	}
	return out[:size], nil
}

// drain discards a reply left over from a command sent without reading.
func (c *Client) drain() {
	if !c.stale {
		return
	}
	c.stale = false
	for i := 0; i < 4; i++ {
		if n, err := c.t.Read(c.buf, 5*time.Millisecond); err != nil || n == 0 {
			return
		}
	}
}

// CleanReply strips the prompt, NUL padding and line endings from a text reply.
func CleanReply(b []byte) string {
	s := strings.ReplaceAll(string(b), "->", "")
	return strings.TrimSpace(strings.Trim(s, "\x00"))
}
