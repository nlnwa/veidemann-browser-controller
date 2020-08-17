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

package screenshotwriter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
)

type screenshotWriterMock struct {}

func NewMock() *screenshotWriterMock {
	return &screenshotWriterMock{}
}

func (c *screenshotWriterMock) Write(_ context.Context, data []byte, metadata Metadata) error {
	b := bytes.NewBuffer(data)
	f, err := os.Create("screenshot.png")
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer func () {
		_ = f.Close()
	}()
	_, err = io.Copy(f, b)
	if err != nil {
		return fmt.Errorf("failed to copy screenshot data to file: %w", err)
	}
	return nil
}

func (c *screenshotWriterMock) Close() {
	_ = os.Remove("screenshot.png")
}
