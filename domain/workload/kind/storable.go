package kind

import (
	"math"
	"strings"
)

// MaxInteger is the largest whole number a resource's spec may hold, as
// whoever asks for one says it: the largest a repository keeps as a whole
// number, an int64. One beyond it is kept by MongoDB as a double, as relaxed
// extended JSON reads it, which reads back as no whole number its kind can
// read, and leaves a record nobody can read; so each kind refuses one where it
// is asked for, as too large, whatever bound of its own it holds it to beside.
const MaxInteger = math.MaxInt64

// Keyable reports whether s can be a key of what a record keeps, as a label's
// is, or a network's a container is aliased on: one with a NUL in it cannot,
// as no key of a document can, and MongoDB's driver refuses the whole record
// for it. Each kind refuses one where it is asked for.
func Keyable(s string) bool {
	return !strings.ContainsRune(s, 0)
}

// ValidLabels reports whether labels can be given as they are: none of their
// keys holds a NUL (Keyable), nor any of their values, which are held to the
// same as their keys, docker being told both as they are.
func ValidLabels(labels map[string]string) bool {
	for key, value := range labels {
		if !Keyable(key) || !Keyable(value) {
			return false
		}
	}

	return true
}
