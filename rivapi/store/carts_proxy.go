package store

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// CartProxy is the rdxport-proxy CartStore.
// Translates REST calls to rdxport.cgi POST requests and parses the XML responses.
// Reference: web/rdxport/carts.cpp, lib/rdcart.cpp:RDCart::xml()
type CartProxy struct {
	rdxportURL string
	client     *http.Client
}

func NewCartProxy(rdxportURL string) *CartProxy {
	return &CartProxy{
		rdxportURL: rdxportURL,
		client:     &http.Client{},
	}
}

// rdxport XML types — field names match lib/rdcart.cpp:RDCart::xml() output

type xmlCartList struct {
	Carts []xmlCart `xml:"cart"`
}

type xmlCart struct {
	Number        uint   `xml:"number"`
	Type          string `xml:"type"`
	GroupName     string `xml:"groupName"`
	Title         string `xml:"title"`
	Artist        string `xml:"artist"`
	Album         string `xml:"album"`
	Year          int    `xml:"year"`
	Label         string `xml:"label"`
	Client        string `xml:"client"`
	Agency        string `xml:"agency"`
	Publisher     string `xml:"publisher"`
	Composer      string `xml:"composer"`
	UserDefined   string `xml:"userDefined"`
	ForcedLength  int    `xml:"forcedLength"`
	AverageLength int    `xml:"averageLength"`
	CutQuantity   uint   `xml:"cutQuantity"`
}

func (p *CartProxy) ListCarts(ctx context.Context, ticket, groupName string) ([]Cart, error) {
	// RDXPORT_COMMAND_LISTCARTS=6; GROUP_NAME optional (web/rdxport/carts.cpp:136)
	params := url.Values{
		"COMMAND": {"6"},
		"TICKET":  {ticket},
	}
	if groupName != "" {
		params.Set("GROUP_NAME", groupName)
	}

	resp, err := p.client.PostForm(p.rdxportURL, params)
	if err != nil {
		return nil, fmt.Errorf("rdxport unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rdxport returned %d", resp.StatusCode)
	}

	var list xmlCartList
	if err := xml.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("parsing rdxport response: %w", err)
	}
	return xmlCartsToStore(list.Carts), nil
}

func (p *CartProxy) GetCart(ctx context.Context, ticket string, number uint) (*Cart, error) {
	// RDXPORT_COMMAND_LISTCART=7; CART_NUMBER required (web/rdxport/carts.cpp:196)
	resp, err := p.client.PostForm(p.rdxportURL, url.Values{
		"COMMAND":     {"7"},
		"TICKET":      {ticket},
		"CART_NUMBER": {strconv.FormatUint(uint64(number), 10)},
	})
	if err != nil {
		return nil, fmt.Errorf("rdxport unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rdxport returned %d", resp.StatusCode)
	}

	var list xmlCartList
	if err := xml.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("parsing rdxport response: %w", err)
	}
	if len(list.Carts) == 0 {
		return nil, nil
	}
	c := xmlCartToStore(list.Carts[0])
	return &c, nil
}

func xmlCartsToStore(xs []xmlCart) []Cart {
	out := make([]Cart, len(xs))
	for i, x := range xs {
		out[i] = xmlCartToStore(x)
	}
	return out
}

func xmlCartToStore(x xmlCart) Cart {
	return Cart{
		Number:        x.Number,
		Type:          x.Type,
		GroupName:     x.GroupName,
		Title:         x.Title,
		Artist:        x.Artist,
		Album:         x.Album,
		Year:          x.Year,
		Label:         x.Label,
		Client:        x.Client,
		Agency:        x.Agency,
		Publisher:     x.Publisher,
		Composer:      x.Composer,
		UserDefined:   x.UserDefined,
		ForcedLength:  x.ForcedLength,
		AverageLength: x.AverageLength,
		CutQuantity:   x.CutQuantity,
	}
}
