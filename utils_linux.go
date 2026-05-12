//go:build linux
// +build linux

package probing

import (
	"errors"
	"os"
	"reflect"
	"syscall"

	"golang.org/x/net/bpf"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Returns the length of an ICMP message.
func (p *Pinger) getMessageLength() int {
	return p.Size + 8
}

// Attempts to match the ID of an ICMP packet.
func (p *Pinger) matchID(ID int) bool {
	// On Linux we can only match ID if we are privileged.
	if p.protocol == "icmp" {
		return ID == p.id
	}
	return true
}

// SetMark sets the SO_MARK socket option on outgoing ICMP packets.
// Setting this option requires CAP_NET_ADMIN.
func (c *icmpConn) SetMark(mark uint) error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, int(mark)),
	)
}

// SetMark sets the SO_MARK socket option on outgoing ICMP packets.
// Setting this option requires CAP_NET_ADMIN.
func (c *icmpv4Conn) SetMark(mark uint) error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, int(mark)),
	)
}

// SetMark sets the SO_MARK socket option on outgoing ICMP packets.
// Setting this option requires CAP_NET_ADMIN.
func (c *icmpV6Conn) SetMark(mark uint) error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, int(mark)),
	)
}

// SetDoNotFragment sets the do-not-fragment bit in the IP header of outgoing ICMP packets.
func (c *icmpConn) SetDoNotFragment() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO),
	)
}

// SetDoNotFragment sets the do-not-fragment bit in the IP header of outgoing ICMP packets.
func (c *icmpv4Conn) SetDoNotFragment() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO),
	)
}

// SetDoNotFragment sets the do-not-fragment bit in the IPv6 header of outgoing ICMPv6 packets.
func (c *icmpV6Conn) SetDoNotFragment() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}
	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_MTU_DISCOVER, syscall.IP_PMTUDISC_DO),
	)
}

func (c *icmpConn) SetBroadcastFlag() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}

	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1),
	)
}

func (c *icmpv4Conn) SetBroadcastFlag() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}

	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1),
	)
}

func (c *icmpV6Conn) SetBroadcastFlag() error {
	fd, err := getFD(c.c)
	if err != nil {
		return err
	}

	return os.NewSyscallError(
		"setsockopt",
		syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1),
	)
}

// InstallICMPIDFilter attaches a BPF program to the connection to filter ICMP packets id.
// Also accepts Time Exceeded messages.
func (c *icmpv4Conn) InstallICMPIDFilter(id int) error {
	filter, err := bpf.Assemble([]bpf.Instruction{
		// Skip IP header (variable length)
		bpf.LoadMemShift{Off: 0},
		// Load ICMP type
		bpf.LoadIndirect{Off: 0, Size: 1},

		// BRANCH 1: Is this packet type Time Exceeded?
		// If yes, ACCEPT immediately. We will filter by ID in Go (User Space).
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(ipv4.ICMPTypeTimeExceeded), SkipTrue: 6, SkipFalse: 0},

		// BRANCH 2: Is this packet an ICMP Echo Reply?
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(ipv4.ICMPTypeEchoReply), SkipTrue: 1, SkipFalse: 0},
		bpf.RetConstant{Val: 0}, // Reject if neither type

		// ACTION: Match ID of the ICMP Echo Reply (Offset 4)
		bpf.LoadIndirect{Off: 4, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(id), SkipTrue: 0, SkipFalse: 1},

		bpf.RetConstant{Val: ^uint32(0)}, // ACCEPT
		bpf.RetConstant{Val: 0},          // REJECT
		bpf.RetConstant{Val: ^uint32(0)}, // ACCEPT (Time Exceeded jump target)
	})
	if err != nil {
		return err
	}
	return c.c.IPv4PacketConn().SetBPF(filter)
}

// InstallICMPIDFilter attaches a BPF program to the connection to filter ICMPv6 packets id.
// Also accepts Time Exceeded messages.
func (c *icmpV6Conn) InstallICMPIDFilter(id int) error {
	filter, err := bpf.Assemble([]bpf.Instruction{
		// Load ICMPv6 type
		bpf.LoadAbsolute{Off: 0, Size: 1},

		// BRANCH 1: Is this packet type Time Exceeded?
		// If yes, ACCEPT immediately. We will filter by ID in Go (User Space).
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(ipv6.ICMPTypeTimeExceeded), SkipTrue: 3, SkipFalse: 0},

		// BRANCH 2: Is this packet an ICMP Echo Reply?
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(ipv6.ICMPTypeEchoReply), SkipTrue: 0, SkipFalse: 3},

		// ACTION: Match ID of the ICMP Echo Reply (Offset 4)
		bpf.LoadAbsolute{Off: 4, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(id), SkipTrue: 0, SkipFalse: 1},

		bpf.RetConstant{Val: ^uint32(0)}, // ACCEPT
		bpf.RetConstant{Val: 0},          // REJECT
	})
	if err != nil {
		return err
	}
	return c.c.IPv6PacketConn().SetBPF(filter)
}

// getFD gets the system file descriptor for an icmp.PacketConn
func getFD(c *icmp.PacketConn) (uintptr, error) {
	v := reflect.ValueOf(c).Elem().FieldByName("c").Elem()
	if v.Elem().Kind() != reflect.Struct {
		return 0, errors.New("invalid type")
	}

	fd := v.Elem().FieldByName("conn").FieldByName("fd")
	if fd.Elem().Kind() != reflect.Struct {
		return 0, errors.New("invalid type")
	}

	pfd := fd.Elem().FieldByName("pfd")
	if pfd.Kind() != reflect.Struct {
		return 0, errors.New("invalid type")
	}

	return uintptr(pfd.FieldByName("Sysfd").Int()), nil
}
