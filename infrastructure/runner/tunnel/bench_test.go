package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// benchConfig is the production defaults with the timings left alone, since
// what is being measured is the data path rather than the pool.
func benchConfig() Config {
	c := DefaultConfig()
	c.MinSessions = 2
	c.MaxSessions = 4
	c.IdleSessionTimeout = 0

	return c
}

// benchTunnel stands an ingress and one worker up, with an echo target.
func benchTunnel(b *testing.B, config Config) (*Ingress, func(context.Context) (net.Conn, error)) {
	b.Helper()

	ingress, err := NewIngress(config, AllowAll(), discardLogger())
	if err != nil {
		b.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() { _ = ingress.Serve(ctx, listener) }()

	target := benchEcho(b)

	worker, err := NewWorker("worker-a", []string{listener.Addr().String()}, config, plainDialer(),
		NewServiceTargets(map[string]string{"echo": target}), discardLogger())
	if err != nil {
		b.Fatal(err)
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		worker.Run(ctx)
	}()

	b.Cleanup(func() {
		cancel()
		worker.Close()
		ingress.Close()
		<-stopped
	})

	// wait for the pool, so the measurement is not of connecting
	deadline := time.Now().Add(10 * time.Second)
	for {
		workers := ingress.Workers()
		if len(workers) == 1 && workers[0].Sessions >= config.MinSessions {
			break
		}

		if time.Now().After(deadline) {
			b.Fatal("the worker never came up")
		}

		time.Sleep(5 * time.Millisecond)
	}

	return ingress, func(ctx context.Context) (net.Conn, error) {
		return ingress.Dial(ctx, "worker-a", Target{Service: "echo"})
	}
}

func benchEcho(b *testing.B) string {
	b.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()

				buffer := make([]byte, 64*1024)
				_, _ = io.CopyBuffer(writerOnly{conn}, readerOnly{conn}, buffer)
			}()
		}
	}()

	b.Cleanup(func() { listener.Close() })

	return listener.Addr().String()
}

// BenchmarkRoundTrip is what one request-and-answer costs on an established
// stream: the latency the tunnel adds to a small exchange.
func BenchmarkRoundTrip(b *testing.B) {
	_, dial := benchTunnel(b, benchConfig())

	conn, err := dial(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	message := make([]byte, 64)
	answer := make([]byte, 64)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := conn.Write(message); err != nil {
			b.Fatal(err)
		}

		if _, err := io.ReadFull(conn, answer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOpenStream is what a new client connection costs, which is the
// handshake's one round trip.
func BenchmarkOpenStream(b *testing.B) {
	config := benchConfig()
	config.MaxStreamsPerSession = 4096

	_, dial := benchTunnel(b, config)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		conn, err := dial(context.Background())
		if err != nil {
			b.Fatal(err)
		}

		conn.Close()
	}
}

// BenchmarkThroughput is how fast one stream moves bytes, which is where the
// stream window shows itself.
func BenchmarkThroughput(b *testing.B) {
	for _, size := range []int{64 * 1024, 256 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprintf("stream-buffer-%dKB", size/1024), func(b *testing.B) {
			config := benchConfig()
			config.MaxStreamBuffer = size
			config.MaxReceiveBuffer = max(size*4, config.MaxReceiveBuffer)

			_, dial := benchTunnel(b, config)

			conn, err := dial(context.Background())
			if err != nil {
				b.Fatal(err)
			}
			defer conn.Close()

			chunk := make([]byte, 64*1024)
			sink := make([]byte, 64*1024)

			b.SetBytes(int64(len(chunk)))
			b.ResetTimer()

			for b.Loop() {
				if _, err := conn.Write(chunk); err != nil {
					b.Fatal(err)
				}

				if _, err := io.ReadFull(conn, sink); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkConcurrentStreams is the shape that matters most: many connections
// at once, all exchanging small messages.
func BenchmarkConcurrentStreams(b *testing.B) {
	for _, streams := range []int{1, 100, 500} {
		b.Run(fmt.Sprintf("streams-%d", streams), func(b *testing.B) {
			config := benchConfig()
			config.MaxStreamsPerSession = 1024

			_, dial := benchTunnel(b, config)

			conns := make([]net.Conn, 0, streams)
			for range streams {
				conn, err := dial(context.Background())
				if err != nil {
					b.Fatal(err)
				}

				conns = append(conns, conn)
			}

			defer func() {
				for _, conn := range conns {
					conn.Close()
				}
			}()

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				var wait sync.WaitGroup

				for _, conn := range conns {
					wait.Add(1)

					go func(conn net.Conn) {
						defer wait.Done()

						message := make([]byte, 64)
						answer := make([]byte, 64)

						if _, err := conn.Write(message); err != nil {
							return
						}

						_, _ = io.ReadFull(conn, answer)
					}(conn)
				}

				wait.Wait()
			}
		})
	}
}

// BenchmarkStreamsPerSession is the tuning question the defaults cannot answer
// on their own: the same load spread over few sessions with many streams each,
// or many sessions with few.
//
// The total is held at 512 streams throughout, so what varies is only how they
// are divided. Fewer, fatter sessions share one congestion window and one write
// path; more, thinner ones cost more sockets and more memory. Where the
// crossover falls depends on the path, which is why the default is a starting
// point rather than an answer.
func BenchmarkStreamsPerSession(b *testing.B) {
	shapes := []struct {
		sessions int
		streams  int
	}{
		{sessions: 1, streams: 512},
		{sessions: 2, streams: 256},
		{sessions: 4, streams: 128},
		{sessions: 8, streams: 64},
		{sessions: 16, streams: 32},
	}

	const total = 64

	for _, shape := range shapes {
		b.Run(fmt.Sprintf("%dx%d", shape.sessions, shape.streams), func(b *testing.B) {
			config := benchConfig()
			config.MinSessions = shape.sessions
			config.MaxSessions = shape.sessions
			config.MaxStreamsPerSession = shape.streams

			_, dial := benchTunnel(b, config)

			conns := make([]net.Conn, 0, total)
			for range total {
				conn, err := dial(context.Background())
				if err != nil {
					b.Fatal(err)
				}

				conns = append(conns, conn)
			}

			defer func() {
				for _, conn := range conns {
					conn.Close()
				}
			}()

			chunk := make([]byte, 32*1024)

			b.SetBytes(int64(len(chunk)) * int64(total))
			b.ResetTimer()

			for b.Loop() {
				var wait sync.WaitGroup

				for _, conn := range conns {
					wait.Add(1)

					go func(conn net.Conn) {
						defer wait.Done()

						sink := make([]byte, len(chunk))

						if _, err := conn.Write(chunk); err != nil {
							return
						}

						_, _ = io.ReadFull(conn, sink)
					}(conn)
				}

				wait.Wait()
			}
		})
	}
}

// BenchmarkManyWorkers is the ingress's side of scale: many workers connected
// at once, each carrying a little.
func BenchmarkManyWorkers(b *testing.B) {
	for _, workers := range []int{1, 10, 50} {
		b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
			config := benchConfig()
			config.MinSessions = 1
			config.MaxSessions = 2

			ingress, err := NewIngress(config, AllowAll(), discardLogger())
			if err != nil {
				b.Fatal(err)
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				b.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			go func() { _ = ingress.Serve(ctx, listener) }()

			target := benchEcho(b)

			var stopped sync.WaitGroup
			names := make([]string, 0, workers)

			for i := range workers {
				name := fmt.Sprintf("worker-%03d", i)
				names = append(names, name)

				worker, err := NewWorker(name, []string{listener.Addr().String()}, config, plainDialer(),
					NewServiceTargets(map[string]string{"echo": target}), discardLogger())
				if err != nil {
					b.Fatal(err)
				}

				stopped.Add(1)

				go func() {
					defer stopped.Done()

					worker.Run(ctx)
				}()
			}

			b.Cleanup(func() {
				cancel()
				ingress.Close()
				stopped.Wait()
			})

			deadline := time.Now().Add(30 * time.Second)
			for len(ingress.Workers()) < workers {
				if time.Now().After(deadline) {
					b.Fatalf("only %d of %d workers came up", len(ingress.Workers()), workers)
				}

				time.Sleep(10 * time.Millisecond)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				for _, name := range names {
					conn, err := ingress.Dial(context.Background(), name, Target{Service: "echo"})
					if err != nil {
						b.Fatal(err)
					}

					conn.Close()
				}
			}
		})
	}
}

// BenchmarkIdleSessions is what holding connections costs when nothing is going
// through them, which is the common state of a tunnel most of the time.
func BenchmarkIdleSessions(b *testing.B) {
	config := benchConfig()
	config.MaxStreamsPerSession = 2048

	_, dial := benchTunnel(b, config)

	conns := make([]net.Conn, 0, 500)
	for range 500 {
		conn, err := dial(context.Background())
		if err != nil {
			b.Fatal(err)
		}

		conns = append(conns, conn)
	}

	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()

	// one stream working while five hundred sit idle
	active := conns[0]
	message := make([]byte, 64)
	answer := make([]byte, 64)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := active.Write(message); err != nil {
			b.Fatal(err)
		}

		if _, err := io.ReadFull(active, answer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRegistry is the one piece of shared state on the path, so it has to
// stay out of the way when many goroutines touch it at once.
func BenchmarkRegistry(b *testing.B) {
	registry := NewRegistry()

	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	muxSession, err := smuxServerOn(right)
	if err != nil {
		b.Fatal(err)
	}
	defer muxSession.Close()

	for i := range 100 {
		if err := registry.Add(newSession(newID(), fmt.Sprintf("worker-%03d", i), muxSession, 256)); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := registry.Sessions("worker-050"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
