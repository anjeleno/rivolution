package store

import (
	"context"
	"database/sql"
)

// GroupDB is the MariaDB-native GroupStore.
// Mirrors web/rdxport/groups.cpp:Xport::ListGroups() and lib/rdgroup.cpp:RDGroup::xml().
type GroupDB struct {
	db *sql.DB
}

func NewGroupDB(db *sql.DB) *GroupDB {
	return &GroupDB{db: db}
}

func (g *GroupDB) ListGroups(ctx context.Context, username string) ([]Group, error) {
	// Users with ADMIN_CONFIG_PRIV='Y' see all groups — same behaviour as RDAdmin,
	// which queries GROUPS directly rather than filtering through USER_PERMS.
	if g.isAdmin(ctx, username) {
		return g.listAllGroups(ctx)
	}
	// mirrors web/rdxport/groups.cpp:46 — permission-filtered group list
	rows, err := g.db.QueryContext(ctx,
		"SELECT `GROUP_NAME` FROM `USER_PERMS` WHERE `USER_NAME` = ? ORDER BY `GROUP_NAME`",
		username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		grp, err := g.fetchGroup(ctx, name)
		if err != nil {
			return nil, err
		}
		if grp != nil {
			groups = append(groups, *grp)
		}
	}
	return groups, rows.Err()
}

func (g *GroupDB) isAdmin(ctx context.Context, username string) bool {
	var priv string
	err := g.db.QueryRowContext(ctx,
		"SELECT COALESCE(`ADMIN_CONFIG_PRIV`,'N') FROM `USERS` WHERE `LOGIN_NAME` = ?",
		username,
	).Scan(&priv)
	return err == nil && priv == "Y"
}

func (g *GroupDB) listAllGroups(ctx context.Context) ([]Group, error) {
	rows, err := g.db.QueryContext(ctx, "SELECT `NAME` FROM `GROUPS` ORDER BY `NAME`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		grp, err := g.fetchGroup(ctx, name)
		if err != nil {
			return nil, err
		}
		if grp != nil {
			groups = append(groups, *grp)
		}
	}
	return groups, rows.Err()
}

func (g *GroupDB) fetchGroup(ctx context.Context, name string) (*Group, error) {
	// mirrors lib/rdgroup.cpp:RDGroup::xml() lines 350-392
	var grp Group
	var cartType uint
	var enforceCartRange, reportTfc, reportMus string

	err := g.db.QueryRowContext(ctx,
		"SELECT COALESCE(`DESCRIPTION`,''), COALESCE(`DEFAULT_CART_TYPE`,0),"+
			" COALESCE(`DEFAULT_LOW_CART`,0), COALESCE(`DEFAULT_HIGH_CART`,0),"+
			" COALESCE(`CUT_SHELFLIFE`,-1), COALESCE(`DEFAULT_TITLE`,''),"+
			" COALESCE(`ENFORCE_CART_RANGE`,'N'), COALESCE(`REPORT_TFC`,'N'),"+
			" COALESCE(`REPORT_MUS`,'N'), COALESCE(`COLOR`,'')"+
			" FROM `GROUPS` WHERE `NAME` = ?",
		name,
	).Scan(
		&grp.Description,
		&cartType,
		&grp.DefaultLowCart,
		&grp.DefaultHighCart,
		&grp.CutShelfLife,
		&grp.DefaultTitle,
		&enforceCartRange,
		&reportTfc,
		&reportMus,
		&grp.Color,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	grp.Name = name
	// RDCart::Type: Audio=1, Macro=2 (lib/rdcart.h:41)
	switch cartType {
	case 1:
		grp.DefaultCartType = "audio"
	case 2:
		grp.DefaultCartType = "macro"
	}
	// Rivendell stores booleans as 'Y'/'N' strings
	grp.EnforceCartRange = enforceCartRange == "Y"
	grp.ReportTfc = reportTfc == "Y"
	grp.ReportMus = reportMus == "Y"
	return &grp, nil
}
