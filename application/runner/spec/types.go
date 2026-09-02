package spec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// StringOrSlice is a compose field that may be written either as one shell
// string or as a list of arguments.
type StringOrSlice []string

var _ json.Unmarshaler = &StringOrSlice{}

func (s *StringOrSlice) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*s = list

		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return fmt.Errorf("expected a string or a list of strings, got %s", data)
	}

	if len(strings.TrimSpace(single)) == 0 {
		*s = nil

		return nil
	}

	*s = strings.Fields(single)

	return nil
}

// Environment is a compose environment, written either as a map or as a list of
// "KEY=value" entries. It normalises to the list docker takes.
type Environment []string

var _ json.Unmarshaler = &Environment{}

func (e *Environment) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*e = list

		return nil
	}

	var pairs map[string]string
	if err := json.Unmarshal(data, &pairs); err != nil {
		return fmt.Errorf("expected a map or a list of KEY=value entries, got %s", data)
	}

	entries := make([]string, 0, len(pairs))
	for key, value := range pairs {
		entries = append(entries, key+"="+value)
	}

	// a map has no order of its own, and a container's environment reads
	// better, and diffs better, sorted.
	sort.Strings(entries)

	*e = entries

	return nil
}

// Port is one entry of a compose ports list. Only the container side matters to
// the runner, which picks the host side itself, but the whole compose syntax is
// accepted so a compose file can be pasted in unchanged.
type Port struct {
	Container port.Port
}

// Ports is a compose ports list.
type Ports []Port

var _ json.Unmarshaler = &Port{}

func (p *Port) UnmarshalJSON(data []byte) error {
	var number uint16
	if err := json.Unmarshal(data, &number); err == nil {
		p.Container = port.Port(number)

		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("expected a port number or a \"host:container\" string, got %s", data)
	}

	container, err := containerPort(text)
	if err != nil {
		return err
	}

	p.Container = container

	return nil
}

// containerPort takes the container side out of compose's port syntax:
// "80", "8080:80", "127.0.0.1:8080:80", and any of those with a "/tcp" suffix.
func containerPort(text string) (port.Port, error) {
	text = strings.TrimSpace(text)

	if protocol := strings.Index(text, "/"); protocol >= 0 {
		text = text[:protocol]
	}

	// the container side is the last colon-separated part, whether the entry
	// names a host port, a host address, or neither.
	if colon := strings.LastIndex(text, ":"); colon >= 0 {
		text = text[colon+1:]
	}

	// a published range is more than the runner offers, and quietly taking the
	// first port of it would not be what was asked for.
	if strings.Contains(text, "-") {
		return 0, fmt.Errorf("a port range is not supported, name the ports one by one")
	}

	number, err := strconv.ParseUint(text, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port", text)
	}

	return port.Port(number), nil
}

// Decimal is a compose number that may be written as a string, the way cpus is.
type Decimal float64

var _ json.Unmarshaler = (*Decimal)(nil)

func (d *Decimal) UnmarshalJSON(data []byte) error {
	var number float64
	if err := json.Unmarshal(data, &number); err == nil {
		*d = Decimal(number)

		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("expected a number or a numeric string, got %s", data)
	}

	if len(strings.TrimSpace(text)) == 0 {
		*d = 0

		return nil
	}

	parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return fmt.Errorf("%q is not a number", text)
	}

	*d = Decimal(parsed)

	return nil
}

// suffixes are the units compose writes a size in. Compose treats them as
// powers of 1024, so "256M" is 256 MiB.
var suffixes = []struct {
	suffix string
	factor int64
}{
	{"gb", 1 << 30},
	{"mb", 1 << 20},
	{"kb", 1 << 10},
	{"g", 1 << 30},
	{"m", 1 << 20},
	{"k", 1 << 10},
	{"b", 1},
}

// ByteSize is a compose size, written either as a plain number of bytes or with
// a unit like "256M".
type ByteSize int64

var _ json.Unmarshaler = (*ByteSize)(nil)

func (b *ByteSize) UnmarshalJSON(data []byte) error {
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		*b = ByteSize(number)

		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("expected a number of bytes or a size like \"256M\", got %s", data)
	}

	size, err := parseByteSize(text)
	if err != nil {
		return err
	}

	*b = size

	return nil
}

func parseByteSize(text string) (ByteSize, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if len(text) == 0 {
		return 0, nil
	}

	for _, unit := range suffixes {
		if !strings.HasSuffix(text, unit.suffix) {
			continue
		}

		amount := strings.TrimSpace(strings.TrimSuffix(text, unit.suffix))

		number, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a size", text)
		}

		return ByteSize(number * float64(unit.factor)), nil
	}

	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size", text)
	}

	return ByteSize(number), nil
}
