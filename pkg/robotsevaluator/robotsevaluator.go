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

package robotsevaluator

import (
	"context"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	"github.com/nlnwa/veidemann-browser-controller/pkg/serviceconnections"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type RobotsEvaluator interface {
	Connect() error
	Close()
	IsAllowed(context.Context, *robotsevaluatorV1.IsAllowedRequest) bool
}

type robotsEvaluator struct {
	opts       []serviceconnections.ConnectionOption
	clientConn *grpc.ClientConn
	client     robotsevaluatorV1.RobotsEvaluatorClient
}

func New(opts ...serviceconnections.ConnectionOption) RobotsEvaluator {
	return &robotsEvaluator{opts: opts}
}

func (r *robotsEvaluator) Connect() error {
	var err error

	if r.clientConn, err = serviceconnections.Connect("RobotsEvaluator", r.opts...); err != nil {
		return err
	}
	r.client = robotsevaluatorV1.NewRobotsEvaluatorClient(r.clientConn)

	return nil
}

func (r *robotsEvaluator) Close() {
	if r.clientConn != nil {
		_ = r.clientConn.Close()
	}
}

func (r *robotsEvaluator) IsAllowed(ctx context.Context, request *robotsevaluatorV1.IsAllowedRequest) bool {
	resolvedPoliteness, ignore := resolvePolicy(request.Politeness)
	if ignore {
		return true
	}

	request.Politeness = resolvedPoliteness
	reply, err := r.client.IsAllowed(ctx, request)
	if err != nil {
		log.Warnf("failed to get allowance from robotsEvaluator: %w", err)
		return true
	}

	return reply.IsAllowed
}

func resolvePolicy(politenessConfig *configV1.ConfigObject) (resolvedPoliteness *configV1.ConfigObject, ignore bool) {
	var resolvedPolicy configV1.PolitenessConfig_RobotsPolicy
	switch politenessConfig.GetPolitenessConfig().GetRobotsPolicy() {
	case configV1.PolitenessConfig_OBEY_ROBOTS_CLASSIC:
		resolvedPolicy = configV1.PolitenessConfig_OBEY_ROBOTS
		break
	case configV1.PolitenessConfig_CUSTOM_ROBOTS_CLASSIC:
		resolvedPolicy = configV1.PolitenessConfig_CUSTOM_ROBOTS
		break
	case configV1.PolitenessConfig_CUSTOM_IF_MISSING_CLASSIC:
		resolvedPolicy = configV1.PolitenessConfig_CUSTOM_IF_MISSING
		break
	default:
		resolvedPolicy = configV1.PolitenessConfig_IGNORE_ROBOTS
		break
	}

	resolvedPoliteness = proto.Clone(politenessConfig).(*configV1.ConfigObject)
	resolvedPoliteness.GetPolitenessConfig().RobotsPolicy = resolvedPolicy
	return resolvedPoliteness, resolvedPolicy == configV1.PolitenessConfig_IGNORE_ROBOTS
}
