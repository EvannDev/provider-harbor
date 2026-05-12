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
