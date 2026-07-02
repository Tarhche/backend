package profiler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdaptiveSamplerRate(t *testing.T) {
	s := newAdaptiveSampler(0.7, 0.1)

	t.Run("full rate below the load threshold", func(t *testing.T) {
		assert.Equal(t, 1.0, s.rate(0))
		assert.Equal(t, 1.0, s.rate(0.7))
	})

	t.Run("scales linearly between threshold and full load", func(t *testing.T) {
		// halfway into the excess range: base - (base-min)*0.5
		assert.InDelta(t, 0.55, s.rate(0.85), 1e-9)
	})

	t.Run("never drops below the minimum rate", func(t *testing.T) {
		assert.InDelta(t, 0.1, s.rate(1), 1e-9)
		assert.InDelta(t, 0.1, s.rate(2), 1e-9)
	})
}

func TestAdaptiveSamplerShouldProfile(t *testing.T) {
	s := newAdaptiveSampler(0.7, 0.1)

	t.Run("profiles when the draw is below the rate", func(t *testing.T) {
		s.rand = func() float64 { return 0.05 }
		assert.True(t, s.shouldProfile(1))
	})

	t.Run("skips when the draw is above the rate", func(t *testing.T) {
		s.rand = func() float64 { return 0.5 }
		assert.False(t, s.shouldProfile(1))
	})
}

func TestOverheadMonitor(t *testing.T) {
	t.Run("stretches the interval when collections are too expensive", func(t *testing.T) {
		m := newOverheadMonitor(time.Minute, 5)

		// 10s of work against a 60s interval is ~17% overhead > 5%
		next := m.observe(10 * time.Second)
		assert.Equal(t, 90*time.Second, next)
	})

	t.Run("caps the interval at ten times the base", func(t *testing.T) {
		m := newOverheadMonitor(time.Minute, 5)

		for range 20 {
			m.observe(time.Hour)
		}
		assert.Equal(t, 10*time.Minute, m.current())
	})

	t.Run("shrinks back to the base interval when there is headroom", func(t *testing.T) {
		m := newOverheadMonitor(time.Minute, 5)
		m.observe(time.Hour) // 90s

		// no measurable cost: shrink again, but never below the base
		for range 20 {
			m.observe(0)
		}
		assert.Equal(t, time.Minute, m.current())
	})

	t.Run("keeps the interval inside the target band", func(t *testing.T) {
		m := newOverheadMonitor(time.Minute, 5)

		// 2s/60s = 3.3%: above half the target, below the target
		next := m.observe(2 * time.Second)
		assert.Equal(t, time.Minute, next)
	})
}

func TestCPUMonitorUsage(t *testing.T) {
	m := newCPUMonitor()

	load := m.usage()
	assert.GreaterOrEqual(t, load, 0.0)
	assert.LessOrEqual(t, load, 1.0)
}
