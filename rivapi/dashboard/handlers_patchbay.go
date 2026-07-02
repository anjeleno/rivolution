package dashboard

import (
	"net/http"
	"net/url"

	"github.com/anjeleno/rivolution/rivapi/store"
)

// patchbayData holds everything the patchbay page needs to render the
// current PipeWire graph as a connect/disconnect matrix.
type patchbayData struct {
	baseData
	Outputs []string
	Inputs  []string
	Linked  map[string]bool // keyed by linkKey(output, input)
	Error   string
}

// linkKey is the composite map key used to look up link state from the
// template (Go templates can't build map keys from two values directly).
func linkKey(output, input string) string {
	return output + "|" + input
}

func (h *Handler) patchbayData() (patchbayData, error) {
	data := patchbayData{baseData: h.base("Patchbay", "patchbay")}

	outputs, err := store.ListOutputPorts()
	if err != nil {
		return data, err
	}
	inputs, err := store.ListInputPorts()
	if err != nil {
		return data, err
	}
	links, err := store.ListPatchLinks()
	if err != nil {
		return data, err
	}

	data.Outputs = outputs
	data.Inputs = inputs
	data.Linked = make(map[string]bool, len(links))
	for _, l := range links {
		data.Linked[linkKey(l.Output, l.Input)] = true
	}
	return data, nil
}

// Patchbay handles GET /patchbay: a live view of every PipeWire output and
// input port and their current links, with a button per pair to connect or
// disconnect it. MVP — no live/auto-refresh yet, reload the page to see
// changes made outside the dashboard (e.g. a client restart dropping links).
func (h *Handler) Patchbay(w http.ResponseWriter, r *http.Request) {
	data, err := h.patchbayData()
	if err != nil {
		data.Error = err.Error()
	} else if e := r.URL.Query().Get("error"); e != "" {
		data.Error = e
	}
	if err := tmplPatchbay.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// PatchbayToggle handles POST /patchbay/toggle: links the given output/input
// pair if not already linked, unlinks it otherwise, then redirects back to
// the page. A plain form POST + redirect, not htmx — kept deliberately
// simple for this first pass; a live/partial-refresh UI is a follow-up.
func (h *Handler) PatchbayToggle(w http.ResponseWriter, r *http.Request) {
	output := r.FormValue("output")
	input := r.FormValue("input")

	data, err := h.patchbayData()
	if err == nil {
		if data.Linked[linkKey(output, input)] {
			err = store.Unlink(output, input)
		} else {
			err = store.Link(output, input)
		}
	}

	redirect := "/patchbay"
	if err != nil {
		redirect += "?error=" + url.QueryEscape(err.Error())
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
