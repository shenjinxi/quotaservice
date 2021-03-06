// Licensed under the Apache License, Version 2.0
// Details: https://raw.githubusercontent.com/maniksurtani/quotaservice/master/LICENSE

package quotaservice

import (
	"net/http"

	"github.com/maniksurtani/quotaservice/config"
	"github.com/maniksurtani/quotaservice/events"
	"github.com/maniksurtani/quotaservice/logging"
	"github.com/maniksurtani/quotaservice/stats"
)

// The Server interface is what you get when you create a new quotaservice.
type Server interface {
	Start() (bool, error)
	Stop() (bool, error)
	SetLogger(logger logging.Logger)
	ServeAdminConsole(*http.ServeMux, string, bool)
	SetListener(listener events.Listener, eventQueueBufSize int)
	SetStatsListener(listener stats.Listener)
}

func NewWithDefaultConfig(bucketFactory BucketFactory, rpcEndpoints ...RpcEndpoint) Server {
	return New(bucketFactory,
		config.NewMemoryConfig(config.NewDefaultServiceConfig()),
		rpcEndpoints...)
}

// New creates a new quotaservice server.
func New(bucketFactory BucketFactory, persister config.ConfigPersister, rpcEndpoints ...RpcEndpoint) Server {
	if len(rpcEndpoints) == 0 {
		panic("Need at least 1 RPC endpoint to run the quota service.")
	}

	s := &server{
		persister:     persister,
		bucketFactory: bucketFactory,
		rpcEndpoints:  rpcEndpoints}
	return s
}
