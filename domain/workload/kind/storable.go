package kind

import "math"

// MaxInteger is the largest whole number a resource's spec may hold, as
// whoever asks for one says it: the largest a repository keeps as a whole
// number, an int64. One beyond it is kept by MongoDB as a double, as relaxed
// extended JSON reads it, which reads back as no whole number its kind can
// read, and leaves a record nobody can read; so each kind refuses one where it
// is asked for, as too large, whatever bound of its own it holds it to beside.
const MaxInteger = math.MaxInt64
