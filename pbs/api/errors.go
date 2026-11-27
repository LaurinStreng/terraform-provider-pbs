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

// IsLockTimeout reports whether err means PBS gave up waiting for a config
// lock, e.g. `Unable to acquire lock "/etc/proxmox-backup/.datastore.lck" -
// Interrupted system call (os error 4)`. PBS waits a few seconds for the lock
// and then aborts with EINTR; the request was not applied, so it is safe to
// retry. Other lock errors (permission denied, "another backup is already
// running", ...) are permanent and deliberately not matched.
func IsLockTimeout(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "unable to acquire lock") &&
		strings.Contains(msg, ".lck") &&
		(strings.Contains(msg, "interrupted system call") || strings.Contains(msg, "os error 4"))
}
