// Licensed under the Apache License, Version 2.0
// Details: https://raw.githubusercontent.com/maniksurtani/quotaservice/master/LICENSE

package quotaservice

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/maniksurtani/quotaservice/admin"
	"github.com/maniksurtani/quotaservice/events"
	"github.com/maniksurtani/quotaservice/lifecycle"
	"github.com/maniksurtani/quotaservice/logging"
	"github.com/maniksurtani/quotaservice/stats"

	proto "github.com/golang/protobuf/proto"
	"github.com/maniksurtani/quotaservice/config"
	pb "github.com/maniksurtani/quotaservice/protos/config"
)

// Implements the quotaservice.Server interface
type server struct {
	currentStatus     lifecycle.Status
	stopper           *chan int
	bucketContainer   *bucketContainer
	bucketFactory     BucketFactory
	rpcEndpoints      []RpcEndpoint
	listener          events.Listener
	statsListener     stats.Listener
	eventQueueBufSize int
	producer          *events.EventProducer
	cfgs              *pb.ServiceConfig
	persister         config.ConfigPersister
	sync.RWMutex      // Embedded mutex
}

func (s *server) String() string {
	return fmt.Sprintf("Quota Server running with status %v", s.currentStatus)
}

func (s *server) Start() (bool, error) {
	// Set up listeners
	s.producer = events.RegisterListener(func(e events.Event) {
		if s.listener != nil {
			s.listener(e)
		}

		if s.statsListener != nil {
			s.statsListener.HandleEvent(e)
		}
	}, s.eventQueueBufSize)

	go s.configListener(s.persister.ConfigChangedWatcher())

	// Start the RPC servers
	for _, rpcServer := range s.rpcEndpoints {
		rpcServer.Init(s)
		rpcServer.Start()
	}

	s.currentStatus = lifecycle.Started
	return true, nil
}

func (s *server) Stop() (bool, error) {
	s.currentStatus = lifecycle.Stopped

	// Stop the RPC servers
	for _, rpcServer := range s.rpcEndpoints {
		rpcServer.Stop()
	}

	return true, nil
}

func (s *server) Allow(namespace, name string, tokensRequested int64, maxWaitMillisOverride int64, maxWaitTimeOverride bool) (time.Duration, error) {
	b, e := s.bucketContainer.FindBucket(namespace, name)
	if e != nil {
		// Attempted to create a dynamic bucket and failed.
		s.Emit(events.NewBucketMissedEvent(namespace, name, true))
		return 0, newError("Cannot create dynamic bucket "+config.FullyQualifiedName(namespace, name), ER_TOO_MANY_BUCKETS)
	}

	if b == nil {
		s.Emit(events.NewBucketMissedEvent(namespace, name, false))
		return 0, newError("No such bucket "+config.FullyQualifiedName(namespace, name), ER_NO_BUCKET)
	}

	if b.Config().MaxTokensPerRequest < tokensRequested && b.Config().MaxTokensPerRequest > 0 {
		s.Emit(events.NewTooManyTokensRequestedEvent(namespace, name, b.Dynamic(), tokensRequested))
		return 0, newError(fmt.Sprintf("Too many tokens requested. Bucket %v:%v, tokensRequested=%v, maxTokensPerRequest=%v",
			namespace, name, tokensRequested, b.Config().MaxTokensPerRequest),
			ER_TOO_MANY_TOKENS_REQUESTED)
	}

	maxWaitTime := time.Millisecond
	if maxWaitTimeOverride {
		// Use the max wait time override from the request.
		maxWaitTime *= time.Duration(maxWaitMillisOverride)
	} else {
		// Fall back to the max wait time configured on the bucket.
		maxWaitTime *= time.Duration(b.Config().WaitTimeoutMillis)
	}

	w, success := b.Take(tokensRequested, maxWaitTime)

	if !success {
		// Could not claim tokens within the given max wait time
		s.Emit(events.NewTimedOutEvent(namespace, name, b.Dynamic(), tokensRequested))
		return 0, newError(fmt.Sprintf("Timed out waiting on %v:%v", namespace, name), ER_TIMEOUT)
	}

	// The only positive result
	s.Emit(events.NewTokensServedEvent(namespace, name, b.Dynamic(), tokensRequested, w))
	return w, nil
}

func (s *server) ServeAdminConsole(mux *http.ServeMux, assetsDir string, development bool) {
	admin.ServeAdminConsole(s, mux, assetsDir, development)
}

func (s *server) SetLogger(logger logging.Logger) {
	if s.currentStatus == lifecycle.Started {
		panic("Cannot set logger after server has started!")
	}
	logging.SetLogger(logger)
}

func (s *server) SetStatsListener(listener stats.Listener) {
	if s.currentStatus == lifecycle.Started {
		panic("Cannot add listener after server has started!")
	}

	s.statsListener = listener
}

func (s *server) SetListener(listener events.Listener, eventQueueBufSize int) {
	if s.currentStatus == lifecycle.Started {
		panic("Cannot add listener after server has started!")
	}

	if eventQueueBufSize < 1 {
		panic("Event queue buffer size must be greater than 0")
	}

	s.listener = listener
	s.eventQueueBufSize = eventQueueBufSize
}

func (s *server) Emit(e events.Event) {
	if s.producer != nil {
		s.producer.Emit(e)
	}
}

func (s *server) configListener(ch chan struct{}) {
	for range ch {
		configReader, err := s.persister.ReadPersistedConfig()

		if err != nil {
			logging.Println("error reading persisted config", err)
		}

		newConfig, err := config.Unmarshal(configReader)

		if err != nil {
			logging.Println("error reading marshalled config", err)
		}

		s.createBucketContainer(newConfig)
	}
}

func (s *server) createBucketContainer(newConfig *pb.ServiceConfig) {
	s.Lock()
	// Initialize buckets
	s.bucketFactory.Init(newConfig)

	s.cfgs = newConfig
	s.bucketContainer = NewBucketContainer(s.cfgs, s.bucketFactory, s)
	s.Unlock()
}

func (s *server) updateConfig(updater func(*pb.ServiceConfig) error) error {
	s.Lock()
	clonedCfg := proto.Clone(s.cfgs).(*pb.ServiceConfig)
	s.Unlock()

	err := updater(clonedCfg)

	if err != nil {
		return err
	}

	r, e := config.Marshal(clonedCfg)

	if e != nil {
		return e
	}

	return s.persister.PersistAndNotify(r)
}

// Implements admin.Administrable
func (s *server) Configs() *pb.ServiceConfig {
	return s.cfgs
}

func (s *server) UpdateConfig(c *pb.ServiceConfig) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		config.ApplyDefaults(c)
		*clonedCfg = *c
		return nil
	})
}

func (s *server) AddBucket(namespace string, b *pb.BucketConfig) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.CreateBucket(clonedCfg, namespace, b)
	})
}

func (s *server) UpdateBucket(namespace string, b *pb.BucketConfig) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.UpdateBucket(clonedCfg, namespace, b)
	})
}

func (s *server) DeleteBucket(namespace, name string) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.DeleteBucket(clonedCfg, namespace, name)
	})
}

func (s *server) AddNamespace(n *pb.NamespaceConfig) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.CreateNamespace(clonedCfg, n)
	})
}

func (s *server) UpdateNamespace(n *pb.NamespaceConfig) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.UpdateNamespace(clonedCfg, n)
	})
}

func (s *server) DeleteNamespace(n string) error {
	return s.updateConfig(func(clonedCfg *pb.ServiceConfig) error {
		return config.DeleteNamespace(clonedCfg, n)
	})
}

func (s *server) TopDynamicHits(namespace string) []*stats.BucketScore {
	if s.statsListener == nil {
		return nil
	}

	return s.statsListener.TopHits(namespace)
}

func (s *server) TopDynamicMisses(namespace string) []*stats.BucketScore {
	if s.statsListener == nil {
		return nil
	}

	return s.statsListener.TopMisses(namespace)
}

func (s *server) DynamicBucketStats(namespace, bucket string) *stats.BucketScores {
	if s.statsListener == nil {
		return nil
	}

	return s.statsListener.Get(namespace, bucket)
}
