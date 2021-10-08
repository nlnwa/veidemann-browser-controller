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

package logger

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	stdLog "log"
	"strings"
)

const (
	FormatterJson   = "json"
	FormatterLogfmt = "logfmt"
)

func InitLog(level, formatter string, logMethod bool) error {
	stdLog.SetOutput(log.StandardLogger().Writer())

	// Configure the log level, defaults to "INFO"
	logLevel, err := log.ParseLevel(level)
	if err != nil {
		return fmt.Errorf("failed to parse log level: %q", level)
	}
	log.SetLevel(logLevel)

	// Configure the log formatter, defaults to ASCII formatter
	switch strings.ToLower(formatter) {
	case FormatterLogfmt:
		log.SetFormatter(&log.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		})
	case FormatterJson:
		log.SetFormatter(&log.JSONFormatter{})
	default:
		return fmt.Errorf("unknown formatter type: %q", formatter)
	}

	if logMethod {
		log.SetReportCaller(true)
	}
	return nil
}
