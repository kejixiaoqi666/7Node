// Modified for xbord-node-v3: stable names and atomic service snapshots.
package vless

import "github.com/sagernet/sing-box/option"

// UpdateUsers publishes one validated credential snapshot. Existing connection
// contexts retain their original identity; removed users cannot authenticate anew.
func (h *Inbound) UpdateUsers(users []option.VLESSUser) error {
	names := make([]string, len(users))
	credentials := make([]string, len(users))
	flows := make([]string, len(users))
	for i, user := range users {
		names[i] = user.Name
		credentials[i] = user.UUID
		flows[i] = user.Flow
	}
	return h.service.UpdateUsers(names, credentials, flows)
}
