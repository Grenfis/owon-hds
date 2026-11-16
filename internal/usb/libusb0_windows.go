//go:build windows

// Package usb opens the HDS272S through libusb0.dll, the libusb-win32 driver
// that OWON's installer sets up, so the original driver stays in place and
// OWON's own software keeps working.
package usb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"owon-hds/internal/hds"
)

const (
	pathMax = 512 // LIBUSB_PATH_MAX in lusb0_usb.h

	// Field offsets in struct usb_bus and struct usb_device on amd64. They do
	// not depend on whether the header packs the structs: all of them follow
	// pointer-sized fields and a 512-byte array.
	busDevicesOffset = 16 + pathMax
	devDescOffset    = 16 + pathMax + 8

	errTimedOut = -116 // -ETIMEDOUT as returned by libusb-win32
)

var (
	dll                = windows.NewLazySystemDLL("libusb0.dll")
	procInit           = dll.NewProc("usb_init")
	procFindBusses     = dll.NewProc("usb_find_busses")
	procFindDevices    = dll.NewProc("usb_find_devices")
	procGetBusses      = dll.NewProc("usb_get_busses")
	procOpen           = dll.NewProc("usb_open")
	procClose          = dll.NewProc("usb_close")
	procSetConfig      = dll.NewProc("usb_set_configuration")
	procClaim          = dll.NewProc("usb_claim_interface")
	procRelease        = dll.NewProc("usb_release_interface")
	procBulkWrite      = dll.NewProc("usb_bulk_write")
	procBulkRead       = dll.NewProc("usb_bulk_read")
	procControlMsg     = dll.NewProc("usb_control_msg")
	procGetStringSimpl = dll.NewProc("usb_get_string_simple")
	procStrerror       = dll.NewProc("usb_strerror")
)

// Device is an open HDS272S. It implements hds.Transport.
type Device struct {
	h       uintptr
	iface   int
	epIn    int
	epOut   int
	Serial  string
	Summary string // endpoints and interface, for the connection log
}

// Open finds the first device with the given IDs and claims its interface.
func Open(vendor, product uint16) (*Device, error) {
	if err := dll.Load(); err != nil {
		return nil, errors.New("не найден libusb0.dll: установите драйвер из папки Driver программы OWON")
	}
	procInit.Call()
	procFindBusses.Call()
	procFindDevices.Call()
	dev, serialIdx := find(vendor, product)
	if dev == 0 {
		return nil, errors.New("прибор не найден: проверьте кабель и режим USB → HID в меню System")
	}
	h, _, _ := procOpen.Call(dev)
	if h == 0 {
		return nil, fmt.Errorf("не удалось открыть прибор: %s", lastError())
	}
	d := &Device{h: h}
	cfg, err := d.readConfig()
	if err != nil {
		procClose.Call(h)
		return nil, err
	}
	if r := call(procSetConfig, h, uintptr(cfg)); r < 0 {
		procClose.Call(h)
		return nil, fmt.Errorf("usb_set_configuration: %s", lastError())
	}
	if r := call(procClaim, h, uintptr(d.iface)); r < 0 {
		procClose.Call(h)
		return nil, fmt.Errorf("прибор занят другой программой (usb_claim_interface: %s)", lastError())
	}
	if serialIdx != 0 {
		buf := make([]byte, 64)
		if n := call(procGetStringSimpl, h, uintptr(serialIdx), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n > 0 {
			d.Serial = string(buf[:n])
		}
	}
	d.Summary = fmt.Sprintf("конфигурация %d, интерфейс %d, IN 0x%02x, OUT 0x%02x", cfg, d.iface, d.epIn, d.epOut)
	return d, nil
}

// find walks libusb's bus and device lists and returns the matching
// struct usb_device pointer and the index of its serial number string.
func find(vendor, product uint16) (uintptr, byte) {
	bus, _, _ := procGetBusses.Call()
	for ; bus != 0; bus = peekPtr(bus) {
		for dev := peekPtr(bus + busDevicesOffset); dev != 0; dev = peekPtr(dev) {
			desc := unsafe.Slice((*byte)(cptr(dev+devDescOffset)), 18)
			if desc[0] == 18 && desc[1] == 1 &&
				binary.LittleEndian.Uint16(desc[8:]) == vendor &&
				binary.LittleEndian.Uint16(desc[10:]) == product {
				return dev, desc[16]
			}
		}
	}
	return 0, 0
}

// readConfig fetches the raw configuration descriptor and picks the first
// interface with a bulk IN and a bulk OUT endpoint, as OWON's software does.
func (d *Device) readConfig() (int, error) {
	buf := make([]byte, 256)
	n := call(procControlMsg, d.h, 0x80, 6, 0x0200, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 1000)
	if n < 9 {
		return 0, fmt.Errorf("не удалось прочитать дескриптор USB: %s", lastError())
	}
	buf = buf[:n]
	cfg, iface := int(buf[5]), -1
scan:
	for i := 0; i+2 <= len(buf) && buf[i] >= 2; i += int(buf[i]) {
		switch desc := buf[i:]; desc[1] {
		case 4: // interface
			if d.epIn != 0 && d.epOut != 0 {
				break scan
			}
			iface, d.epIn, d.epOut = int(desc[2]), 0, 0
		case 5: // endpoint
			if len(desc) < 4 || desc[3]&3 != 2 {
				continue
			}
			if desc[2]&0x80 != 0 {
				d.epIn = int(desc[2])
			} else {
				d.epOut = int(desc[2])
			}
		}
	}
	if iface < 0 || d.epIn == 0 || d.epOut == 0 {
		return 0, errors.New("у прибора нет bulk-точек: проверьте режим USB → HID в меню System")
	}
	d.iface = iface
	return cfg, nil
}

func (d *Device) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := call(procBulkWrite, d.h, uintptr(d.epOut), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), 1000)
	if n < 0 {
		return 0, fmt.Errorf("запись в USB: %s", lastError())
	}
	return n, nil
}

func (d *Device) Read(p []byte, timeout time.Duration) (int, error) {
	n := call(procBulkRead, d.h, uintptr(d.epIn), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), uintptr(max(1, timeout.Milliseconds())))
	switch {
	case n == errTimedOut:
		return 0, hds.ErrTimeout
	case n < 0:
		return 0, fmt.Errorf("чтение из USB: %s", lastError())
	}
	return n, nil
}

func (d *Device) Close() error {
	procRelease.Call(d.h, uintptr(d.iface))
	procClose.Call(d.h)
	return nil
}

// call invokes a libusb function that returns a C int.
func call(p *windows.LazyProc, args ...uintptr) int {
	r, _, _ := p.Call(args...)
	return int(int32(r))
}

func peekPtr(addr uintptr) uintptr {
	return *(*uintptr)(cptr(addr))
}

// cptr turns an address inside libusb's own heap into a pointer. That memory
// is not managed by Go, so the address stays valid; the indirection only keeps
// go vet from flagging the uintptr conversion.
func cptr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func lastError() string {
	p, _, _ := procStrerror.Call()
	if p == 0 {
		return "неизвестная ошибка"
	}
	return windows.BytePtrToString((*byte)(cptr(p)))
}
