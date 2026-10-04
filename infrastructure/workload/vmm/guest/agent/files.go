package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// the files the agent keeps for the task, by their name in its own
// directory, and where they appear inside the task's root.
var boundFiles = map[string]string{
	"hosts":       "/etc/hosts",
	"resolv.conf": "/etc/resolv.conf",
	"hostname":    "/etc/hostname",
}

// writeFiles writes the files the agent keeps for the task into dir.
func writeFiles(dir string, config guest.Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	files := map[string][]byte{
		"hosts":       hostsFile(config.Hostname, config.Interfaces, config.Hosts),
		"resolv.conf": resolvConf(config.Nameservers),
		"hostname":    []byte(config.Hostname + "\n"),
	}

	for name, content := range files {
		if err := writeInPlace(filepath.Join(dir, name), content); err != nil {
			return err
		}
	}

	return nil
}

// writeInPlace rewrites a file without replacing it. A file bound elsewhere is
// bound by what it is rather than by its name, so one written anew would leave
// the binding showing the old one.
func writeInPlace(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	if _, err := file.Write(content); err != nil {
		file.Close()

		return err
	}

	return file.Close()
}

// hostsFile is the machine's /etc/hosts: itself, under its own name on each of
// its addresses, and its neighbours under theirs.
func hostsFile(hostname string, interfaces []guest.Interface, hosts []guest.Host) []byte {
	var content strings.Builder

	content.WriteString("127.0.0.1\tlocalhost\n")
	content.WriteString("::1\tlocalhost ip6-localhost ip6-loopback\n")

	for _, i := range interfaces {
		address, _, _ := strings.Cut(i.Address, "/")
		if len(address) > 0 && len(hostname) > 0 {
			fmt.Fprintf(&content, "%s\t%s\n", address, hostname)
		}
	}

	for _, host := range hosts {
		if len(host.Address) > 0 && len(host.Names) > 0 {
			fmt.Fprintf(&content, "%s\t%s\n", host.Address, strings.Join(host.Names, " "))
		}
	}

	return []byte(content.String())
}

// resolvConf is the machine's /etc/resolv.conf. A machine that cannot reach
// the internet is given no nameserver, so a lookup fails at once rather than
// waiting on one it cannot reach.
func resolvConf(nameservers []string) []byte {
	var content strings.Builder

	for _, nameserver := range nameservers {
		fmt.Fprintf(&content, "nameserver %s\n", nameserver)
	}

	return []byte(content.String())
}

// primaryAddress is the address the task's ports are reached on: its own on
// the first network it joined, which is the one its neighbours reach it on.
func primaryAddress(interfaces []guest.Interface) (string, bool) {
	if len(interfaces) == 0 {
		return "", false
	}

	address, _, _ := strings.Cut(interfaces[0].Address, "/")

	return address, len(address) > 0
}
