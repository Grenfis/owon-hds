//go:build !windows

package usb

import (
	"errors"
	"time"
)

// Device is a stub: the USB driver is only available on Windows.
type Device struct {
	Serial  string
	Summary string
}

func Open(vendor, product uint16) (*Device, error) {
	return nil, errors.New("USB работает только в Windows; здесь доступен имитатор")
}

func (d *Device) Write(p []byte) (int, error)                       { return 0, errors.ErrUnsupported }
func (d *Device) Read(p []byte, timeout time.Duration) (int, error) { return 0, errors.ErrUnsupported }
func (d *Device) Close() error                                      { return nil }
