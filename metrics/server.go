/*
 * Copyright 2020 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package metrics

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
)

// Server is the Prometheus metrics endpoint for the Browser Controller
type Server struct {
	*http.Server
}

// NewServer returns a new instance of Server listening on the given port
func NewServer(listenInterface string, listenPort int, path string) *Server {
	a := &Server{
		Server: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", listenInterface, listenPort),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  5 * time.Second,
		},
	}
	http.Handle(path, promhttp.Handler())

	return a
}

func (a *Server) Start() error {
	log.Info().Str("address", a.Addr).Msg("Metrics server")
	if err := a.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}

func (a *Server) Close() error {
	log.Info().Msg("Shutting down Metrics server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.SetKeepAlivesEnabled(false)
	return a.Shutdown(ctx)
}
