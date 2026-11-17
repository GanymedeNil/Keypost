package importer

import (
	"context"
	"errors"
	"io"
	"strings"
)

// stageOrder lists the Lemon Squeezy resources in dependency order.
var stageOrder = []string{
	"stores", "customers", "products", "variants", "prices", "files",
	"orders", "order_items", "subscriptions", "subscription_items", "subscription_invoices",
	"license_keys", "license_key_instances",
}

// storeFiltered lists the resources the Lemon Squeezy API can filter by
// filter[store_id].
var storeFiltered = map[string]bool{
	"customers": true, "products": true, "orders": true, "subscriptions": true,
	"subscription_invoices": true, "license_keys": true,
}

// resourceGroups expands a selected resource to the resources that belong
// to it (selecting "orders" imports its order items too).
var resourceGroups = map[string][]string{
	"orders":        {"orders", "order_items"},
	"subscriptions": {"subscriptions", "subscription_items", "subscription_invoices"},
	"license_keys":  {"license_keys", "license_key_instances"},
}

var resourceAliases = map[string]string{
	"store": "stores", "customer": "customers", "product": "products", "variant": "variants", "price": "prices",
	"file": "files", "order": "orders", "order_item": "order_items", "subscription": "subscriptions",
	"subscription_item": "subscription_items", "subscription_invoice": "subscription_invoices",
	"license_key": "license_keys", "license_key_instance": "license_key_instances",
	"license_keys_instances": "license_key_instances", "license_instances": "license_key_instances",
}

// normalizeResourceName maps "license-keys", "License_Key" ... to the canonical name.
func normalizeResourceName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("-", "_", " ", "_").Replace(s)
	if a, ok := resourceAliases[s]; ok {
		return a
	}
	return s
}

func apiPath(res string) string { return "/v1/" + strings.ReplaceAll(res, "_", "-") }

// errNoFileData means a source cannot provide the bytes of a file.
var errNoFileData = errors.New("no file data available")

type fileRef struct {
	Identifier string
	Name       string
	URL        string // signed download URL, may be empty
}

// source produces the resource objects of an import.
type source interface {
	// has reports whether the source can provide the resource.
	has(res string) bool
	// stream feeds every object of the resource to h.
	stream(ctx context.Context, res string, h *streamHandler) error
	// openFile returns the bytes of a product file or errNoFileData.
	openFile(ctx context.Context, f fileRef) (io.ReadCloser, error)
}
