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

package controller

import (
	"context"
	configV1 "github.com/nlnwa/veidemann-api-go/config/v1"
	robotsevaluatorV1 "github.com/nlnwa/veidemann-api-go/robotsevaluator/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func (bc *BrowserController) funcIsAllowed(ctx context.Context, request *robotsevaluatorV1.IsAllowedRequest) bool {
	resolvedPoliteness, ignore := bc.resolvePolicy(request.Politeness)
	if ignore {
		return true
	}

	request.Politeness = resolvedPoliteness
	reply, err := bc.opts.robotsEvaluatorConn.Client().IsAllowed(ctx, request)
	if err != nil {
		log.Errorf("Failed to open RobotsEvaluator session: %v", err)
		return true
	}

	return reply.IsAllowed
}

func (bc *BrowserController) resolvePolicy(politenessConfig *configV1.ConfigObject) (resolvedPoliteness *configV1.ConfigObject, ignore bool) {
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
