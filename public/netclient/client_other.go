//go:build !linux

// Copyright 2025 Redpanda Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package netclient

// SetTCPUserTimeout does not apply to non-linux systems as it is not
// supported on other platforms. Errors and warnings will be silently ignored.
func (c *Config) setTCPUserTimeout(_ int) error {
	return nil
}
