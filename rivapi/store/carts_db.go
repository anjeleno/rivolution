package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// CartDB is the MariaDB-native cart reader.
// Used for admin users who bypass rdxport's USER_PERMS filter.
// Mirrors the field set returned by lib/rdcart.cpp:RDCart::xml().
type CartDB struct {
	db *sql.DB
}

func NewCartDB(db *sql.DB) *CartDB {
	return &CartDB{db: db}
}

const cartSelectCols = "NUMBER, TYPE, GROUP_NAME, " +
	"COALESCE(TITLE,''), COALESCE(ARTIST,''), COALESCE(ALBUM,''), YEAR, " +
	"COALESCE(LABEL,''), COALESCE(CLIENT,''), COALESCE(AGENCY,''), " +
	"COALESCE(PUBLISHER,''), COALESCE(COMPOSER,''), COALESCE(USER_DEFINED,''), " +
	"COALESCE(FORCED_LENGTH,0), COALESCE(AVERAGE_LENGTH,0), COALESCE(CUT_QUANTITY,0)"

func (c *CartDB) ListCarts(ctx context.Context, groupName string) ([]Cart, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if groupName != "" {
		rows, err = c.db.QueryContext(ctx,
			"SELECT "+cartSelectCols+" FROM CART WHERE GROUP_NAME = ? ORDER BY NUMBER",
			groupName)
	} else {
		rows, err = c.db.QueryContext(ctx,
			"SELECT "+cartSelectCols+" FROM CART ORDER BY NUMBER")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var carts []Cart
	for rows.Next() {
		cart, err := scanCart(rows)
		if err != nil {
			return nil, err
		}
		carts = append(carts, cart)
	}
	return carts, rows.Err()
}

func (c *CartDB) GetCart(ctx context.Context, number uint) (*Cart, error) {
	row := c.db.QueryRowContext(ctx,
		"SELECT "+cartSelectCols+" FROM CART WHERE NUMBER = ?", number)
	cart, err := scanCartRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

// rowScanner abstracts *sql.Rows and *sql.Row so scanCart can serve both.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCart(s rowScanner) (Cart, error) {
	var (
		cartType             int64
		forcedMs, averageMs  int64
		yearDate             sql.NullTime
		cart                 Cart
	)
	err := s.Scan(
		&cart.Number, &cartType, &cart.GroupName,
		&cart.Title, &cart.Artist, &cart.Album, &yearDate,
		&cart.Label, &cart.Client, &cart.Agency,
		&cart.Publisher, &cart.Composer, &cart.UserDefined,
		&forcedMs, &averageMs, &cart.CutQuantity,
	)
	if err != nil {
		return Cart{}, err
	}
	cart.Type = cartTypeStr(cartType)
	if yearDate.Valid {
		cart.Year = strconv.Itoa(yearDate.Time.Year())
	}
	cart.ForcedLength = msToHHMMSSt(forcedMs)
	cart.AverageLength = msToHHMMSSt(averageMs)
	return cart, nil
}

func scanCartRow(r *sql.Row) (Cart, error) {
	return scanCart(r)
}

// cartTypeStr converts Rivendell's RDCart::Type enum int to the string
// rdxport serializes in XML: Audio=1 → "audio", Macro=2 → "macro".
func cartTypeStr(t int64) string {
	switch t {
	case 1:
		return "audio"
	case 2:
		return "macro"
	default:
		return fmt.Sprintf("%d", t)
	}
}

// msToHHMMSSt converts a millisecond duration to the "HH:MM:SS.T" format
// rdxport uses in its XML output (T = tenths of second).
func msToHHMMSSt(ms int64) string {
	tenths := (ms / 100) % 10
	secs := (ms / 1000) % 60
	mins := (ms / 60000) % 60
	hours := ms / 3600000
	return fmt.Sprintf("%02d:%02d:%02d.%d", hours, mins, secs, tenths)
}
