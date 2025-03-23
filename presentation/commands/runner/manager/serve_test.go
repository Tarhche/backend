package manager

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serve-runner-manager"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serves a http server."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serve-runner-manager [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewServeCommand(nil)

		flagSet := flag.NewFlagSet(command.Name(), flag.ContinueOnError)

		command.Configure(flagSet)

		port := flagSet.Lookup("port")
		if port == nil {
			t.Fatal("port flag has not been configured")
		}

		if port.Usage != "specifies which port server should listen to." {
			t.Error("unexpected port flag usage")
		}

		if command.port != 80 {
			t.Error("unexpected port flag default value")
		}

		if err := flagSet.Parse([]string{"-port", "100"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.port != 100 {
			t.Error("unexpected port flag default value")
		}
	})

	t.Run("run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
			fmt.Fprint(rw, "test response")
		})

		subscribers := map[string]domain.MessageHandler{
			"test1": domain.MessageHandlerFunc(func(message []byte) error { return nil }),
			"test2": domain.MessageHandlerFunc(func(message []byte) error { return nil }),
			"test3": domain.MessageHandlerFunc(func(message []byte) error { return nil }),
		}

		var subscriber messaging.MockPublishSubscriber
		subscriber.On("Subscribe", ctx, consumerName, mock.Anything, mock.Anything).Times(len(subscribers)).Return(nil)
		defer subscriber.AssertExpectations(t)

		var serviceProvider ioc.ServiceProviderMock
		defer serviceProvider.AssertNotCalled(t, "Register")
		defer serviceProvider.AssertNotCalled(t, "Boot")
		defer serviceProvider.AssertNotCalled(t, "Terminate")

		command := NewServeCommand(&serviceProvider)
		command.port = 1234
		command.handler = handler
		command.subscriber = &subscriber
		command.subscribers = subscribers

		serverStartedListening := make(chan struct{})

		go func() {
			serverStartedListening <- struct{}{}
			command.Run(ctx)
		}()

		<-serverStartedListening
		time.Sleep(50 * time.Millisecond) // wait for server to start serving

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://0.0.0.0:%d", command.port), nil)
		assert.NoError(t, err)

		c := http.Client{
			Timeout: 1 * time.Second,
		}

		resp, err := c.Do(req)

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

	})
}
