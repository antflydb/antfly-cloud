package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Table methods use a client whose base URL is the instance connection ProxyURL.
// Only named-table routes are exposed, so table-scoped readers stay read-only.
func tableAPIPath(table string) (string, error) {
	if strings.TrimSpace(table) == "" || table == "." || table == ".." || strings.ContainsAny(table, "/\\") {
		return "", errors.New("an explicit table name without path separators is required")
	}
	return "/db/v1/tables/" + url.PathEscape(table), nil
}

func (c *Client) QueryTable(ctx context.Context, table string, req json.RawMessage) (json.RawMessage, error) {
	path, err := tableAPIPath(table)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	if err := c.requestJSON(ctx, http.MethodPost, path+"/query", req, &result, false); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetTable(ctx context.Context, table string) (json.RawMessage, error) {
	path, err := tableAPIPath(table)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, &result, false); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) DropTable(ctx context.Context, table string) error {
	path, err := tableAPIPath(table)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path, nil, nil, false)
}
