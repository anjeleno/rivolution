package store

import "context"

// Group mirrors the fields returned by lib/rdgroup.cpp:RDGroup::xml().
type Group struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	DefaultCartType  string `json:"defaultCartType,omitempty"`
	DefaultLowCart   uint   `json:"defaultLowCart"`
	DefaultHighCart  uint   `json:"defaultHighCart"`
	CutShelfLife     int    `json:"cutShelfLife"`
	DefaultTitle     string `json:"defaultTitle"`
	EnforceCartRange bool   `json:"enforceCartRange"`
	ReportTfc        bool   `json:"reportTfc"`
	ReportMus        bool   `json:"reportMus"`
	Color            string `json:"color"`
}

// Cart mirrors the core cart-level fields from lib/rdcart.cpp:RDCart::xml().
// Full field list: lib/rdcart.cpp:1464 (xmlSql). Cut fields excluded from Phase 1.
// ForcedLength and AverageLength are rdxport time strings ("MM:SS.S"), not integers.
type Cart struct {
	Number        uint   `json:"number"`
	Type          string `json:"type"`
	GroupName     string `json:"groupName"`
	Title         string `json:"title"`
	Artist        string `json:"artist"`
	Album         string `json:"album"`
	Year          string `json:"year,omitempty"`
	Label         string `json:"label"`
	Client        string `json:"client"`
	Agency        string `json:"agency"`
	Publisher     string `json:"publisher"`
	Composer      string `json:"composer"`
	UserDefined   string `json:"userDefined"`
	ForcedLength  string `json:"forcedLength"`
	AverageLength string `json:"averageLength"`
	CutQuantity   uint   `json:"cutQuantity"`
}

// GroupStore is the interface for group data access.
// The MariaDB-native implementation is in groups_db.go.
type GroupStore interface {
	ListGroups(ctx context.Context, username string) ([]Group, error)
	IsAdmin(ctx context.Context, username string) bool
}

// CartStore is the interface for cart data access.
// The rdxport proxy implementation is in carts_proxy.go.
type CartStore interface {
	ListCarts(ctx context.Context, ticket, groupName string) ([]Cart, error)
	GetCart(ctx context.Context, ticket string, number uint) (*Cart, error)
}
