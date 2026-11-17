// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package probing

import (
	"errors"
	"net"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// boundDevice reads SO_BINDTODEVICE back from the socket behind sc.
func boundDevice(t *testing.T, sc syscall.Conn) string {
	t.Helper()
	rc, err := sc.SyscallConn()
	AssertNoError(t, err)
	var name string
	var sockErr error
	AssertNoError(t, rc.Control(func(fd uintptr) {
		name, sockErr = unix.GetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE)
	}))
	AssertNoError(t, sockErr)
	return name
}

// skipIfNotPermitted fails the test on err, except for EPERM: kernels older than
// 5.7 refuse SO_BINDTODEVICE on an unprivileged ICMP datagram socket.
func skipIfNotPermitted(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, syscall.EPERM) {
		t.Skipf("SO_BINDTODEVICE not permitted in this environment: %v", err)
	}
	AssertNoError(t, err)
}

func loopbackName(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	AssertNoError(t, err)
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 && i.Flags&net.FlagUp != 0 {
			return i.Name
		}
	}
	t.Skip("no loopback interface")
	return ""
}

func TestBindToDeviceSetsSocketOption(t *testing.T) {
	lo := loopbackName(t)
	p := New("127.0.0.1")
	p.SetPrivileged(false)
	AssertNoError(t, p.Resolve())
	conn, err := p.listen()
	if err != nil {
		t.Skipf("cannot open ICMP socket: %v", err)
	}
	defer conn.Close()

	skipIfNotPermitted(t, conn.BindToDevice(lo))
	sc := conn.(*icmpv4Conn).c.IPv4PacketConn().PacketConn.(syscall.Conn)
	if got := boundDevice(t, sc); got != lo {
		t.Fatalf("SO_BINDTODEVICE = %q, want %q", got, lo)
	}
}

func TestBindToDeviceUnknownDeviceFailsBeforeSending(t *testing.T) {
	p := New("127.0.0.1")
	p.SetPrivileged(false)
	p.Count = 1
	p.Timeout = time.Second
	p.BindToDevice = "nosuchdev0"
	var sent int
	p.OnSend = func(*Packet) { sent++ }
	err := p.Run()
	if err == nil {
		t.Fatal("expected an error for an unknown device")
	}
	if !errors.Is(err, syscall.ENODEV) && !errors.Is(err, syscall.EPERM) {
		t.Skipf("socket could not be opened in this environment: %v", err)
	}
	AssertTrue(t, sent == 0)
	AssertTrue(t, p.PacketsSent == 0)
}

func TestBindToDeviceLoopbackPing(t *testing.T) {
	lo := loopbackName(t)
	for _, privileged := range []bool{false, true} {
		t.Run(map[bool]string{false: "dgram", true: "raw"}[privileged], func(t *testing.T) {
			p := New("127.0.0.1")
			p.SetPrivileged(privileged)
			p.Count = 2
			p.Interval = 50 * time.Millisecond
			p.Timeout = 2 * time.Second
			p.BindToDevice = lo
			if err := p.Run(); err != nil {
				t.Skipf("cannot run in this environment: %v", err)
			}
			AssertTrue(t, p.PacketsSent == 2)
			AssertTrue(t, p.PacketsRecv == 2)
		})
	}
}

func TestBindToDeviceIPv6(t *testing.T) {
	lo := loopbackName(t)
	p := New("::1")
	p.SetPrivileged(false)
	AssertNoError(t, p.Resolve())
	conn, err := p.listen()
	if err != nil {
		t.Skipf("no IPv6 ICMP socket: %v", err)
	}
	defer conn.Close()
	skipIfNotPermitted(t, conn.BindToDevice(lo))
	sc := conn.(*icmpV6Conn).c.IPv6PacketConn().PacketConn.(syscall.Conn)
	if got := boundDevice(t, sc); got != lo {
		t.Fatalf("SO_BINDTODEVICE = %q, want %q", got, lo)
	}
}
