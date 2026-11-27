/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package api

import "strings"

// IsNotFound reports whether err indicates that a PBS object does not exist.
// PBS is inconsistent here: depending on the endpoint it answers with 404 or
// with 400/500 and a message such as "no such datastore 'x'".
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "status 404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such")
}

// IsLockTimeout reports whether err means PBS could not acquire a config
// lock in time (e.g. `Unable to acquire lock "/etc/proxmox-backup/.datastore.lck"
// - Interrupted system call (os error 4)`). PBS gives up after a few seconds
// when another task or API call holds the lock. The request was not applied,
// so it is safe to retry.
func IsLockTimeout(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "unable to acquire lock") ||
		(strings.Contains(msg, ".lck") && strings.Contains(msg, "interrupted system call"))
}
