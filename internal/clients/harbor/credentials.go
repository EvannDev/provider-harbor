// Copyright 2026 The Crossplane Authors.
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

package harbor

import "encoding/json"

// Credentials holds the Harbor username and password extracted from a Kubernetes Secret.
// The Secret value must be a JSON object with "username" and "password" keys:
//
//	{"username": "admin", "password": "Harbor12345"}
//
// For robot accounts, use the robot account name as username (e.g. "robot$myrobot")
// and its token as password.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ParseCredentials deserializes the raw JSON bytes from a Kubernetes Secret into Credentials.
func ParseCredentials(data []byte) (Credentials, error) {
	var creds Credentials

	return creds, json.Unmarshal(data, &creds)
}
