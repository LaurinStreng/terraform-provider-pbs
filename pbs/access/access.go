/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

// Package access provides API client functionality for PBS users, API tokens
// and ACLs (/access/...).
package access

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

// Client represents the access API client.
type Client struct {
	api *api.Client
}

// NewClient creates a new access API client.
func NewClient(apiClient *api.Client) *Client {
	return &Client{api: apiClient}
}

// User represents a PBS user (/access/users/{userid}).
//
// PBS omits default values in responses: a missing Enable means true, a
// missing Expire means 0 (never).
type User struct {
	UserID    string `json:"userid"`
	Comment   string `json:"comment,omitempty"`
	Enable    *bool  `json:"enable,omitempty"`
	Expire    *int64 `json:"expire,omitempty"`
	Firstname string `json:"firstname,omitempty"`
	Lastname  string `json:"lastname,omitempty"`
	Email     string `json:"email,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

// UserUpdate holds the fields sent on PUT /access/users/{userid}. Nil/empty
// fields are not sent. Delete accepts comment, firstname, lastname, email.
type UserUpdate struct {
	Comment   *string
	Enable    *bool
	Expire    *int64
	Firstname *string
	Lastname  *string
	Email     *string
	Delete    []string
}

// APIToken represents a PBS API token (/access/users/{userid}/token/{name}).
type APIToken struct {
	TokenID   string `json:"tokenid"`
	TokenName string `json:"token-name,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Enable    *bool  `json:"enable,omitempty"`
	Expire    *int64 `json:"expire,omitempty"`
}

// APITokenUpdate holds the fields sent on PUT .../token/{name}. Delete accepts
// only "comment".
type APITokenUpdate struct {
	Comment *string
	Enable  *bool
	Expire  *int64
	Delete  []string
}

// ACLEntry is one PBS ACL entry. The unique key is (Path, UGID, RoleID).
type ACLEntry struct {
	Path      string `json:"path"`
	UGID      string `json:"ugid"`
	UGIDType  string `json:"ugid_type"`
	Propagate bool   `json:"propagate"`
	RoleID    string `json:"roleid"`
}

// Role is a built-in PBS role.
type Role struct {
	RoleID  string   `json:"roleid"`
	Privs   []string `json:"privs"`
	Comment string   `json:"comment,omitempty"`
}

// Realm is an authentication domain (/access/domains).
type Realm struct {
	Realm   string `json:"realm"`
	Type    string `json:"type"`
	Default *bool  `json:"default,omitempty"`
	Comment string `json:"comment,omitempty"`
}

func userPath(userid string) string {
	return "/access/users/" + url.PathEscape(userid)
}

func tokenPath(userid, tokenName string) string {
	return userPath(userid) + "/token/" + url.PathEscape(tokenName)
}

// ---------------------------------------------------------------- users

// ListUsers lists all users visible to the caller.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	resp, err := c.api.Get(ctx, "/access/users")
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	var users []User
	if err := json.Unmarshal(resp.Data, &users); err != nil {
		return nil, fmt.Errorf("failed to unmarshal users: %w", err)
	}
	return users, nil
}

// GetUser reads a single user.
func (c *Client) GetUser(ctx context.Context, userid string) (*User, error) {
	resp, err := c.api.Get(ctx, userPath(userid))
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", userid, err)
	}
	var user User
	if err := json.Unmarshal(resp.Data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user %s: %w", userid, err)
	}
	if user.Digest == "" {
		user.Digest = resp.Digest
	}
	if user.UserID == "" {
		user.UserID = userid
	}
	return &user, nil
}

// CreateUser creates a user. password may be empty (user cannot log in with
// the pbs realm then).
func (c *Client) CreateUser(ctx context.Context, user *User, password string) error {
	if user.UserID == "" {
		return fmt.Errorf("userid is required")
	}
	body := map[string]interface{}{"userid": user.UserID}
	if user.Comment != "" {
		body["comment"] = user.Comment
	}
	if user.Enable != nil {
		body["enable"] = *user.Enable
	}
	if user.Expire != nil {
		body["expire"] = *user.Expire
	}
	if user.Firstname != "" {
		body["firstname"] = user.Firstname
	}
	if user.Lastname != "" {
		body["lastname"] = user.Lastname
	}
	if user.Email != "" {
		body["email"] = user.Email
	}
	if password != "" {
		body["password"] = password
	}
	if _, err := c.api.Post(ctx, "/access/users", body); err != nil {
		return fmt.Errorf("failed to create user %s: %w", user.UserID, err)
	}
	return nil
}

// UpdateUser updates a user. Passwords cannot be changed here (PBS silently
// ignores them); use ChangePassword.
func (c *Client) UpdateUser(ctx context.Context, userid string, upd *UserUpdate) error {
	body := map[string]interface{}{}
	if upd.Comment != nil {
		body["comment"] = *upd.Comment
	}
	if upd.Enable != nil {
		body["enable"] = *upd.Enable
	}
	if upd.Expire != nil {
		body["expire"] = *upd.Expire
	}
	if upd.Firstname != nil {
		body["firstname"] = *upd.Firstname
	}
	if upd.Lastname != nil {
		body["lastname"] = *upd.Lastname
	}
	if upd.Email != nil {
		body["email"] = *upd.Email
	}
	if len(upd.Delete) > 0 {
		body["delete"] = upd.Delete
	}
	if _, err := c.api.Put(ctx, userPath(userid), body); err != nil {
		return fmt.Errorf("failed to update user %s: %w", userid, err)
	}
	return nil
}

// DeleteUser deletes a user together with its API tokens (and their ACLs).
// ACL entries of the user itself are NOT removed by PBS.
func (c *Client) DeleteUser(ctx context.Context, userid string) error {
	if _, err := c.api.Delete(ctx, userPath(userid)); err != nil {
		return fmt.Errorf("failed to delete user %s: %w", userid, err)
	}
	return nil
}

// ChangePassword sets a user's password via PUT /access/password. PBS
// requires the caller's own password as confirmation unless the caller is
// root@pam, and rejects API-token callers entirely.
func (c *Client) ChangePassword(ctx context.Context, userid, password string) error {
	if c.api.UsesAPIToken() {
		return fmt.Errorf("PBS does not allow changing passwords when authenticated with an API token; " +
			"configure the provider with username/password")
	}
	body := map[string]interface{}{
		"userid":   userid,
		"password": password,
	}
	if c.api.Username() != "root@pam" {
		body["confirmation-password"] = c.api.LoginPassword()
	}
	if _, err := c.api.Put(ctx, "/access/password", body); err != nil {
		return fmt.Errorf("failed to change password of user %s: %w", userid, err)
	}
	return nil
}

// ---------------------------------------------------------------- tokens

// ListAPITokens lists the API tokens of a user.
func (c *Client) ListAPITokens(ctx context.Context, userid string) ([]APIToken, error) {
	resp, err := c.api.Get(ctx, userPath(userid)+"/token")
	if err != nil {
		return nil, fmt.Errorf("failed to list API tokens of user %s: %w", userid, err)
	}
	var tokens []APIToken
	if err := json.Unmarshal(resp.Data, &tokens); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API tokens: %w", err)
	}
	return tokens, nil
}

// GetAPIToken reads a single API token. The secret is never returned.
func (c *Client) GetAPIToken(ctx context.Context, userid, tokenName string) (*APIToken, error) {
	resp, err := c.api.Get(ctx, tokenPath(userid, tokenName))
	if err != nil {
		return nil, fmt.Errorf("failed to get API token %s!%s: %w", userid, tokenName, err)
	}
	var token APIToken
	if err := json.Unmarshal(resp.Data, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API token: %w", err)
	}
	if token.TokenName == "" {
		token.TokenName = tokenName
	}
	if token.TokenID == "" {
		token.TokenID = userid + "!" + tokenName
	}
	return &token, nil
}

// CreateAPIToken creates an API token and returns its secret, which PBS
// reveals only once.
func (c *Client) CreateAPIToken(ctx context.Context, userid, tokenName string, token *APIToken) (string, error) {
	body := map[string]interface{}{}
	if token != nil {
		if token.Comment != "" {
			body["comment"] = token.Comment
		}
		if token.Enable != nil {
			body["enable"] = *token.Enable
		}
		if token.Expire != nil {
			body["expire"] = *token.Expire
		}
	}
	resp, err := c.api.Post(ctx, tokenPath(userid, tokenName), body)
	if err != nil {
		return "", fmt.Errorf("failed to create API token %s!%s: %w", userid, tokenName, err)
	}
	var out struct {
		TokenID string `json:"tokenid"`
		Value   string `json:"value"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return "", fmt.Errorf("failed to unmarshal API token create response: %w", err)
	}
	return out.Value, nil
}

// UpdateAPIToken updates an API token.
func (c *Client) UpdateAPIToken(ctx context.Context, userid, tokenName string, upd *APITokenUpdate) error {
	body := map[string]interface{}{}
	if upd.Comment != nil {
		body["comment"] = *upd.Comment
	}
	if upd.Enable != nil {
		body["enable"] = *upd.Enable
	}
	if upd.Expire != nil {
		body["expire"] = *upd.Expire
	}
	if len(upd.Delete) > 0 {
		body["delete"] = upd.Delete
	}
	if _, err := c.api.Put(ctx, tokenPath(userid, tokenName), body); err != nil {
		return fmt.Errorf("failed to update API token %s!%s: %w", userid, tokenName, err)
	}
	return nil
}

// RegenerateAPIToken issues a new secret for an existing token (PBS >= 3.3.7).
// ACLs of the token are kept.
func (c *Client) RegenerateAPIToken(ctx context.Context, userid, tokenName string) (string, error) {
	resp, err := c.api.Put(ctx, tokenPath(userid, tokenName), map[string]interface{}{"regenerate": true})
	if err != nil {
		return "", fmt.Errorf("failed to regenerate API token %s!%s: %w", userid, tokenName, err)
	}
	var out struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return "", fmt.Errorf("failed to unmarshal API token regenerate response: %w", err)
	}
	return out.Secret, nil
}

// DeleteAPIToken deletes an API token and its ACL entries.
func (c *Client) DeleteAPIToken(ctx context.Context, userid, tokenName string) error {
	if _, err := c.api.Delete(ctx, tokenPath(userid, tokenName)); err != nil {
		return fmt.Errorf("failed to delete API token %s!%s: %w", userid, tokenName, err)
	}
	return nil
}

// ---------------------------------------------------------------- ACLs

// ListACL returns ACL entries. With exact=true only entries on exactly path
// are returned. An empty path lists everything.
func (c *Client) ListACL(ctx context.Context, path string, exact bool) ([]ACLEntry, error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	if exact {
		q.Set("exact", "1")
	}
	p := "/access/acl"
	if enc := q.Encode(); enc != "" {
		p += "?" + enc
	}
	resp, err := c.api.Get(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("failed to list ACLs: %w", err)
	}
	var entries []ACLEntry
	if err := json.Unmarshal(resp.Data, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ACLs: %w", err)
	}
	return entries, nil
}

// GetACL returns the ACL entry (path, authID, role), or an error matched by
// api.IsNotFound when it does not exist.
func (c *Client) GetACL(ctx context.Context, path, authID, role string) (*ACLEntry, error) {
	entries, err := c.ListACL(ctx, path, true)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		e := entries[i]
		if e.UGIDType == "user" && e.UGID == authID && e.RoleID == role {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("ACL entry %s on %s for %s does not exist", role, path, authID)
}

// SetACL adds or overwrites an ACL entry (re-adding updates propagate).
func (c *Client) SetACL(ctx context.Context, path, authID, role string, propagate bool) error {
	body := map[string]interface{}{
		"path":      path,
		"auth-id":   authID,
		"role":      role,
		"propagate": propagate,
	}
	if _, err := c.api.Put(ctx, "/access/acl", body); err != nil {
		return fmt.Errorf("failed to set ACL %s on %s for %s: %w", role, path, authID, err)
	}
	return nil
}

// DeleteACL removes an ACL entry. PBS does not fail if it is missing.
func (c *Client) DeleteACL(ctx context.Context, path, authID, role string) error {
	body := map[string]interface{}{
		"path":    path,
		"auth-id": authID,
		"role":    role,
		"delete":  true,
	}
	if _, err := c.api.Put(ctx, "/access/acl", body); err != nil {
		return fmt.Errorf("failed to delete ACL %s on %s for %s: %w", role, path, authID, err)
	}
	return nil
}

// ---------------------------------------------------------------- roles / realms

// ListRoles lists the built-in roles.
func (c *Client) ListRoles(ctx context.Context) ([]Role, error) {
	resp, err := c.api.Get(ctx, "/access/roles")
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	var roles []Role
	if err := json.Unmarshal(resp.Data, &roles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}
	return roles, nil
}

// ListRealms lists the authentication realms.
func (c *Client) ListRealms(ctx context.Context) ([]Realm, error) {
	resp, err := c.api.Get(ctx, "/access/domains")
	if err != nil {
		return nil, fmt.Errorf("failed to list realms: %w", err)
	}
	var realms []Realm
	if err := json.Unmarshal(resp.Data, &realms); err != nil {
		return nil, fmt.Errorf("failed to unmarshal realms: %w", err)
	}
	return realms, nil
}
