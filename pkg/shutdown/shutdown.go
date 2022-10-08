package shutdown

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

type (
	CallbackFunc func(ctx context.Context)

	GracefulShutdown struct {
		ctx       context.Context
		mu        *sync.RWMutex
		callbacks []CallbackFunc
		done      chan struct{}
	}
)

func NewGracefulShutdown() *GracefulShutdown {
	osCTX, cancel := signal.NotifyContext(context.Background(), signals...)

	shutdown := &GracefulShutdown{
		ctx:       osCTX,
		mu:        new(sync.RWMutex),
		callbacks: make([]CallbackFunc, 0),
		done:      make(chan struct{}),
	}

	go func() {
		defer cancel()

		<-shutdown.ctx.Done()

		shutdown.ForceShutdown()
	}()

	return shutdown
}

func (s *GracefulShutdown) Add(fn CallbackFunc) {
	zap.S().Debugf("added shutdown callback")

	s.mu.Lock()
	s.callbacks = append(s.callbacks, fn)
	s.mu.Unlock()
}

func (s *GracefulShutdown) ForceShutdown() {
	defer close(s.done)

	ctx, cancel := context.WithTimeout(context.Background(), Timeout())
	defer cancel()

	s.mu.RLock()
	callbacks := s.callbacks
	s.mu.RUnlock()

	wg := new(sync.WaitGroup)
	wg.Add(len(callbacks))

	for _, callback := range callbacks {
		threadSafeCallback := callback

		go func() {
			defer wg.Done()

			threadSafeCallback(ctx)
		}()
	}

	wg.Wait()
}

func (s *GracefulShutdown) Context() context.Context {
	return s.ctx
}

func (s *GracefulShutdown) Wait() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, signals...)

	select {
	case <-s.done:
		return nil
	case <-stop:
		return fmt.Errorf("shutdown force stopped")
	}
}
