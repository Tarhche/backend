package runs

import (
	"strconv"
	"strings"
	"syscall"
)

// Signals are numbered as Linux numbers them, whatever the service was built
// for, because they are delivered inside a Linux guest. syscall's own
// constants are the build's, and differ on darwin for all but a few, which is
// why none of them is used here.
const (
	sigTerm = syscall.Signal(15)
	sigKill = syscall.Signal(9)

	sigRTMin = 34
	sigRTMax = 64
)

// linuxSignals are the signals an image's STOPSIGNAL may name.
var linuxSignals = map[string]syscall.Signal{
	"HUP":    1,
	"INT":    2,
	"QUIT":   3,
	"ILL":    4,
	"TRAP":   5,
	"ABRT":   6,
	"IOT":    6,
	"BUS":    7,
	"FPE":    8,
	"KILL":   9,
	"USR1":   10,
	"SEGV":   11,
	"USR2":   12,
	"PIPE":   13,
	"ALRM":   14,
	"TERM":   15,
	"STKFLT": 16,
	"CHLD":   17,
	"CONT":   18,
	"STOP":   19,
	"TSTP":   20,
	"TTIN":   21,
	"TTOU":   22,
	"URG":    23,
	"XCPU":   24,
	"XFSZ":   25,
	"VTALRM": 26,
	"PROF":   27,
	"WINCH":  28,
	"IO":     29,
	"POLL":   29,
	"PWR":    30,
	"SYS":    31,
}

// parseSignal reads a signal the way docker reads an image's STOPSIGNAL: by
// number, or by name with or without its SIG prefix, real-time signals
// included. It reports false for anything else.
func parseSignal(value string) (syscall.Signal, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))

	if number, err := strconv.Atoi(value); err == nil {
		if number < 1 || number > sigRTMax {
			return 0, false
		}

		return syscall.Signal(number), true
	}

	value = strings.TrimPrefix(value, "SIG")

	if signal, found := linuxSignals[value]; found {
		return signal, true
	}

	return parseRealTimeSignal(value)
}

// parseRealTimeSignal reads RTMIN, RTMIN+n, RTMAX and RTMAX-n.
func parseRealTimeSignal(value string) (syscall.Signal, bool) {
	var (
		base     int
		operator string
		rest     string
	)

	switch {
	case strings.HasPrefix(value, "RTMIN"):
		base, operator, rest = sigRTMin, "+", strings.TrimPrefix(value, "RTMIN")
	case strings.HasPrefix(value, "RTMAX"):
		base, operator, rest = sigRTMax, "-", strings.TrimPrefix(value, "RTMAX")
	default:
		return 0, false
	}

	number := base

	if len(rest) > 0 {
		digits, found := strings.CutPrefix(rest, operator)

		offset, err := strconv.Atoi(digits)
		if !found || err != nil || offset < 0 {
			return 0, false
		}

		if operator == "+" {
			number += offset
		} else {
			number -= offset
		}
	}

	if number < sigRTMin || number > sigRTMax {
		return 0, false
	}

	return syscall.Signal(number), true
}

// stopSignalOf is the signal a run's main process is stopped with: its
// image's STOPSIGNAL, or SIGTERM when the image names none, or one that is no
// signal.
func stopSignalOf(image ImageConfig) syscall.Signal {
	if len(image.StopSignal) == 0 {
		return sigTerm
	}

	if signal, ok := parseSignal(image.StopSignal); ok {
		return signal
	}

	return sigTerm
}
