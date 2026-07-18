package dashboard

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/anjeleno/rivolution/rivapi/store"
)

// patchLinkView is one row in the "current connections" list — a live link,
// annotated with whether it's also part of the saved (persistent) set.
type patchLinkView struct {
	Output string
	Input  string
	Saved  bool
}

// patchbayData holds everything the patchbay page needs: the current
// connection list (for the "current connections" section) and the full
// port lists (for the "add connection" dropdowns).
//
// Layout note: this used to be an output x input matrix table. Replaced
// 2026-07-01 — with more than a handful of ports the matrix didn't fit on
// screen at a readable zoom level. A connections-list + add-connection-
// dropdowns layout scales with the number of *connections* (usually small)
// rather than outputs x inputs (which grows fast), and reads as plain text
// instead of a wide grid.
type patchbayData struct {
	baseData
	Outputs           []string
	Inputs            []string
	Links             []patchLinkView
	Error             string
	JustSaved         bool // true right after a successful Save, for a confirmation message
	DisconnectedCount int  // > 0 right after a successful bulk disconnect-unsaved

	// ProgramSource: what feeds every broadcast stream. Moved here from
	// /broadcast (2026-07-17) -- it's a routing decision, not a stream
	// encoding setting, and belongs next to the rest of this station's
	// patching. Still just BroadcastConfig.ProgramSource under the hood
	// (see its own doc comment) and still only takes effect on a stream
	// once /broadcast's "Save & Deploy" runs -- saving it here only
	// updates the stored config value, deliberately not an immediate
	// redeploy from a page that isn't otherwise deploy-triggering.
	ProgramSource        string
	ProgramSourceOptions []string
	ProgramSourceSaved   bool // true right after a successful Program Source save
}

func linkKey(output, input string) string {
	return output + "|" + input
}

func (h *Handler) patchbayData(r *http.Request) (patchbayData, error) {
	data := patchbayData{baseData: h.base(r, "Patchbay", "patchbay")}

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
	saved, err := store.LoadDesiredLinks(store.DesiredLinksPath)
	if err != nil {
		return data, err
	}

	savedSet := make(map[string]bool, len(saved))
	for _, l := range saved {
		savedSet[linkKey(l.Output, l.Input)] = true
	}

	data.Outputs = outputs
	data.Inputs = inputs
	data.Links = make([]patchLinkView, len(links))
	for i, l := range links {
		data.Links[i] = patchLinkView{
			Output: l.Output,
			Input:  l.Input,
			Saved:  savedSet[linkKey(l.Output, l.Input)],
		}
	}

	// Best-effort, same tolerance as the rest of this page doesn't have the
	// luxury of (the live port lists above ARE the point of /patchbay): a
	// config-load hiccup or an empty client list shouldn't block loading
	// everything else. Client names, not individual ports -- ProgramSource
	// is a per-client concept (see ListOutputClients), unlike this page's
	// own per-port Outputs/Inputs dropdowns.
	if cfg, err := store.LoadBroadcastConfig(h.cfg.BroadcastConfigPath); err == nil {
		data.ProgramSource = cfg.ProgramSource
	}
	data.ProgramSourceOptions, _ = store.ListOutputClients()

	return data, nil
}

// Patchbay handles GET /patchbay.
func (h *Handler) Patchbay(w http.ResponseWriter, r *http.Request) {
	data, err := h.patchbayData(r)
	if err != nil {
		data.Error = err.Error()
	} else if e := r.URL.Query().Get("error"); e != "" {
		data.Error = e
	}
	data.JustSaved = r.URL.Query().Get("saved") == "1"
	data.ProgramSourceSaved = r.URL.Query().Get("program_source_saved") == "1"
	if n, err := strconv.Atoi(r.URL.Query().Get("disconnected")); err == nil {
		data.DisconnectedCount = n
	}
	if err := tmplPatchbay.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// PatchbayProgramSource handles POST /patchbay/program-source: updates just
// BroadcastConfig.ProgramSource and saves the config. Deliberately does not
// touch icecast.xml, ffmpeg streams, or any running service -- unlike
// /broadcast's "Save & Deploy", this only records the new value; it takes
// effect the next time a stream is (re)deployed from /broadcast. See
// patchbayData's ProgramSource doc comment for why.
func (h *Handler) PatchbayProgramSource(w http.ResponseWriter, r *http.Request) {
	redirect := "/patchbay"

	cfg, err := store.LoadBroadcastConfig(h.cfg.BroadcastConfigPath)
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	cfg.ProgramSource = strings.TrimSpace(r.FormValue("program_source"))
	if err := store.SaveBroadcastConfig(cfg, h.cfg.BroadcastConfigPath); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, redirect+"?program_source_saved=1", http.StatusSeeOther)
}

// PatchbaySave handles POST /patchbay/save: persists the current live link
// set as the desired set, so the background reconciler (main.go) re-applies
// it after any endpoint restarts. Deliberately snapshots the *live* graph
// rather than accepting an arbitrary posted list — what you see connected
// right now is what gets saved, no separate "staged" state to track.
func (h *Handler) PatchbaySave(w http.ResponseWriter, r *http.Request) {
	links, err := store.ListPatchLinks()
	redirect := "/patchbay"
	if err == nil {
		err = store.SaveDesiredLinks(links, store.DesiredLinksPath)
	}
	if err != nil {
		redirect += "?error=" + url.QueryEscape(err.Error())
	} else {
		redirect += "?saved=1"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// PatchbayDisconnectUnsaved handles POST /patchbay/disconnect-unsaved — a
// bulk cleanup for the common case of a pile of auto-detected connections
// (WirePlumber's own default auto-linking, or a device like Stereo Tool's
// ALSA/JACK driver probing several instances while its I/O is configured)
// that all need removing at once, rather than one "Remove" click per row.
func (h *Handler) PatchbayDisconnectUnsaved(w http.ResponseWriter, r *http.Request) {
	redirect := "/patchbay"
	removed, err := store.DisconnectUnsaved(store.DesiredLinksPath)
	if err != nil {
		redirect += "?error=" + url.QueryEscape(err.Error())
	} else {
		redirect += "?disconnected=" + url.QueryEscape(fmt.Sprintf("%d", removed))
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// PatchbayConnect handles POST /patchbay/connect — the "Add connection" form.
func (h *Handler) PatchbayConnect(w http.ResponseWriter, r *http.Request) {
	output := r.FormValue("output")
	input := r.FormValue("input")

	redirect := "/patchbay"
	if output == "" || input == "" {
		redirect += "?error=" + url.QueryEscape("choose both an output and an input")
	} else if err := store.Link(output, input); err != nil {
		redirect += "?error=" + url.QueryEscape(err.Error())
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// PatchbayDisconnect handles POST /patchbay/disconnect — a "Remove" button
// on one row of the current-connections list.
func (h *Handler) PatchbayDisconnect(w http.ResponseWriter, r *http.Request) {
	output := r.FormValue("output")
	input := r.FormValue("input")

	redirect := "/patchbay"
	if err := store.Unlink(output, input); err != nil {
		redirect += "?error=" + url.QueryEscape(err.Error())
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
