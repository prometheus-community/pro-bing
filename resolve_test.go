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

package probing

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestRunWithContextResolveCancellation(t *testing.T) {
	for _, resolveTimeout := range []time.Duration{0, time.Hour} {
		t.Run(resolveTimeout.String(), func(t *testing.T) {
			for _, phase := range []string{"before lookup", "during lookup", "deadline"} {
				t.Run(phase, func(t *testing.T) {
					started, stopped, release := blockTestResolver(t)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					wantErr := context.Canceled
					switch phase {
					case "before lookup":
						cancel()
					case "deadline":
						var cancelDeadline context.CancelFunc
						ctx, cancelDeadline = context.WithTimeout(ctx, 100*time.Millisecond)
						defer cancelDeadline()
						wantErr = context.DeadlineExceeded
					}

					p := New("cancel.pro-bing.test.")
					p.SetNetwork("ip4")
					p.ResolveTimeout = resolveTimeout
					result := make(chan error, 1)
					go func() { result <- p.RunWithContext(ctx) }()
					if phase != "before lookup" {
						select {
						case <-started:
						case <-time.After(time.Second):
							t.Fatal("DNS lookup did not start")
						}
					}
					if phase == "during lookup" {
						cancel()
					}
					select {
					case err := <-result:
						if !errors.Is(err, wantErr) {
							t.Errorf("RunWithContext returned %v, want %v", err, wantErr)
						}
					case <-time.After(time.Second):
						t.Error("RunWithContext did not stop DNS resolution after cancellation")
						release()
						select {
						case <-result:
						case <-time.After(time.Second):
							t.Fatal("RunWithContext did not stop after releasing the resolver")
						}
					}
					select {
					case <-started:
						select {
						case <-stopped:
						case <-time.After(time.Second):
							t.Error("DNS resolver did not observe cancellation")
						}
					default:
					}
					if p.IPAddr() != nil {
						t.Errorf("canceled lookup stored IP address %v", p.IPAddr())
					}
				})
			}
		})
	}
}

func TestResolveContextTimeout(t *testing.T) {
	_, stopped, _ := blockTestResolver(t)
	p := New("timeout.pro-bing.test.")
	p.SetNetwork("ip4")
	p.ResolveTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.resolve(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("resolve returned %v, want deadline exceeded", err)
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("resolve canceled its parent context: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Error("DNS resolver did not observe ResolveTimeout")
	}
}

func TestResolveAddressSelection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		addresses   []string
		network     string
		first, last string
	}{
		{"mixed", []string{"192.0.2.1", "192.0.2.2", "2001:db8::1", "2001:db8::2"}, "ip", "192.0.2.1", "192.0.2.2"},
		{"ipv4", []string{"192.0.2.1", "192.0.2.2", "2001:db8::1"}, "ip4", "192.0.2.1", "192.0.2.2"},
		{"ipv6", []string{"192.0.2.1", "2001:db8::1", "2001:db8::2"}, "ip6", "2001:db8::1", "2001:db8::1"},
		{"ipv6 only", []string{"2001:db8::1", "2001:db8::2"}, "ip", "2001:db8::1", "2001:db8::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ips []net.IP
			for _, address := range tc.addresses {
				ips = append(ips, net.ParseIP(address))
			}
			setTestResolver(t, func(context.Context, string, string) (net.Conn, error) {
				return &testDNSConn{ips: ips}, nil
			})
			for _, timeout := range []time.Duration{0, time.Hour} {
				want := tc.first
				if timeout > 0 {
					want = tc.last
				}
				for _, withContext := range []bool{false, true} {
					p := New("addresses.pro-bing.test.")
					p.SetNetwork(tc.network)
					p.ResolveTimeout = timeout
					ctx, cancel := context.WithCancel(context.Background())
					var err error
					if withContext {
						err = p.resolve(ctx)
					} else {
						err = p.Resolve()
					}
					cancel()
					AssertNoError(t, err)
					if got := p.IPAddr().String(); got != want {
						t.Errorf("timeout=%v, context=%v: got %s, want %s", timeout, withContext, got, want)
					}
				}
			}
		})
	}
}

func TestResolveLiteralAddresses(t *testing.T) {
	setTestResolver(t, func(context.Context, string, string) (net.Conn, error) {
		t.Error("literal address triggered DNS resolution")
		return nil, errors.New("unexpected DNS lookup")
	})
	for _, tc := range []struct {
		address, network string
		want, wantTimed  string
	}{
		{"127.0.0.1", "ip", "127.0.0.1", "127.0.0.1"},
		{"127.0.0.1", "ip6", "", ""},
		{"::1", "ip", "::1", "::1"},
		{"::1", "ip4", "", ""},
		{"::", "ip", "::", "0.0.0.0"},
		{"::", "ip4", "0.0.0.0", "0.0.0.0"},
		{"::ffff:192.0.2.1", "ip", "192.0.2.1", "192.0.2.1"},
		{"fe80::1%test-zone", "ip", "fe80::1%test-zone", "fe80::1"},
		{"fe80::1%7", "ip6", "fe80::1%7", "fe80::1"},
	} {
		t.Run(tc.address+"/"+tc.network, func(t *testing.T) {
			for _, timeout := range []time.Duration{0, time.Hour} {
				p := New(tc.address)
				p.SetNetwork(tc.network)
				p.ResolveTimeout = timeout
				want := tc.want
				if timeout > 0 {
					want = tc.wantTimed
				}
				err := p.resolve(t.Context())
				if want == "" {
					AssertError(t, err, "address family mismatch")
					continue
				}
				AssertNoError(t, err)
				AssertEqualStrings(t, want, p.IPAddr().String())
			}
		})
	}
	p, err := NewPinger("fe80::1%test-zone")
	AssertNoError(t, err)
	AssertEqualStrings(t, "fe80::1%test-zone", p.IPAddr().String())
	AssertNoError(t, p.SetAddr("fe80::2%7"))
	AssertEqualStrings(t, "fe80::2%7", p.IPAddr().String())
}

type testDNSConn struct {
	response bytes.Reader
	ips      []net.IP
}

func (c *testDNSConn) Read(b []byte) (int, error) { return c.response.Read(b) }

func (c *testDNSConn) Write(b []byte) (int, error) {
	var request dnsmessage.Message
	if err := request.Unpack(b[2:]); err != nil {
		return 0, err
	}
	response := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: request.ID, Response: true, Authoritative: true},
		Questions: request.Questions,
	}
	for _, q := range request.Questions {
		for _, ip := range c.ips {
			header := dnsmessage.ResourceHeader{Name: q.Name, Type: q.Type, Class: q.Class}
			if ip4 := ip.To4(); ip4 != nil && q.Type == dnsmessage.TypeA {
				response.Answers = append(response.Answers, dnsmessage.Resource{
					Header: header,
					Body:   &dnsmessage.AResource{A: [4]byte(ip4)},
				})
			} else if ip.To4() == nil && q.Type == dnsmessage.TypeAAAA {
				response.Answers = append(response.Answers, dnsmessage.Resource{
					Header: header,
					Body:   &dnsmessage.AAAAResource{AAAA: [16]byte(ip.To16())},
				})
			}
		}
	}
	data, err := response.Pack()
	if err != nil {
		return 0, err
	}
	c.response.Reset(append(binary.BigEndian.AppendUint16(nil, uint16(len(data))), data...))
	return len(b), nil
}

func (*testDNSConn) Close() error                     { return nil }
func (*testDNSConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*testDNSConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*testDNSConn) SetDeadline(time.Time) error      { return nil }
func (*testDNSConn) SetReadDeadline(time.Time) error  { return nil }
func (*testDNSConn) SetWriteDeadline(time.Time) error { return nil }

func blockTestResolver(t *testing.T) (started, stopped <-chan struct{}, release func()) {
	t.Helper()
	start := make(chan struct{})
	stop := make(chan struct{})
	unblock := make(chan struct{})
	var startOnce, stopOnce, releaseOnce sync.Once
	release = func() { releaseOnce.Do(func() { close(unblock) }) }
	setTestResolver(t, func(ctx context.Context, _, _ string) (net.Conn, error) {
		startOnce.Do(func() { close(start) })
		defer stopOnce.Do(func() { close(stop) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-unblock:
			return nil, errors.New("test resolver released")
		}
	})
	t.Cleanup(release)
	return start, stop, release
}

func setTestResolver(t *testing.T, dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: dial}
	t.Cleanup(func() { net.DefaultResolver = previous })
}
