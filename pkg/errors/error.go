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

package errors

import (
	"fmt"
	commonsV1 "github.com/nlnwa/veidemann-api-go/commons/v1"
)

type FetchError interface {
	CommonsError() *commonsV1.Error
	Error() string
}

func New(code int32, msg, detail string) FetchError {
	return &fetchError{err: commonsV1.Error{
		Code:   code,
		Msg:    msg,
		Detail: detail,
	}}
}

type fetchError struct {
	err commonsV1.Error
}

func (fe *fetchError) Error() string {
	return fmt.Sprintf("%d %s: %s", fe.err.Code, fe.err.Msg, fe.err.Detail)
}

func (fe *fetchError) CommonsError() *commonsV1.Error {
	return &fe.err
}

func CommonsError(err error) *commonsV1.Error {
	if fe, ok := err.(*fetchError); ok {
		return fe.CommonsError()
	} else {
		return &commonsV1.Error{
			Code:   -5,
			Msg:    "Runtime exception",
			Detail: err.Error(),
		}
	}
}
