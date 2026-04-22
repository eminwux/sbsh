// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/eminwux/sbsh/pkg/api"
	"gopkg.in/yaml.v3"
)

// LoadConfigurationDoc reads a YAML file and returns the first Configuration
// document it contains. Returns (nil, nil) when the file does not exist or
// when the file exists but contains no Configuration document, so callers can
// fall back to built-in defaults. Returns a non-nil error only when the file
// is present but malformed or uses an unsupported apiVersion/kind.
func LoadConfigurationDoc(path string) (*api.ConfigurationDoc, error) {
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open config file %q: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	for {
		var doc api.ConfigurationDoc
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}
			return nil, fmt.Errorf("decode config file %q: %w", path, err)
		}

		if doc.APIVersion == "" && doc.Kind == "" {
			continue
		}

		if doc.Kind != api.KindConfiguration {
			return nil, fmt.Errorf(
				"config file %q: unsupported kind %q (expected %q)",
				path, doc.Kind, api.KindConfiguration,
			)
		}

		return &doc, nil
	}
}
