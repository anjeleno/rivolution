package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/anjeleno/rivolution/rivapi/store"
)

type exportPageData struct {
	baseData
	ImportError  string
	Steps        []string
	JustImported bool
}

// Export handles GET /export — the export/import page itself.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	data := exportPageData{baseData: h.base(r, "Backup", "export")}
	if e := r.URL.Query().Get("error"); e != "" {
		data.ImportError = e
	}
	if err := tmplExport.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// ExportDownload handles GET /export/download — serves the current
// configuration bundle as a downloadable JSON file. Contains real secrets
// (Icecast passwords, and a remote MySQL password if this station is in
// client mode) -- served with Content-Disposition: attachment so browsers
// download rather than display it, but the file itself is exactly as
// sensitive as /etc/rd.conf or the broadcast config already are.
func (h *Handler) ExportDownload(w http.ResponseWriter, r *http.Request) {
	bundle, err := store.BuildExportBundle(h.cfg.BroadcastConfigPath)
	if err != nil {
		http.Error(w, "error building export: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		http.Error(w, "error encoding export: "+err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("rivolution-export-%s.json", time.Now().Format("2026-01-02-1504"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(data)
}

// ExportImport handles POST /export/import — accepts an uploaded bundle
// file and restores it. Always re-renders the full page with a
// step-by-step log, same reasoning as BroadcastSave/ModeApply: an operator
// restoring a station needs to see exactly how far this got.
func (h *Handler) ExportImport(w http.ResponseWriter, r *http.Request) {
	redirect := "/export"

	file, _, err := r.FormFile("bundle")
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("no file uploaded"), http.StatusSeeOther)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	var bundle store.ExportBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("not a valid export file: "+err.Error()), http.StatusSeeOther)
		return
	}

	steps, err := store.ApplyImportBundle(bundle, h.cfg.BroadcastConfigPath)

	page := exportPageData{
		baseData: h.base(r, "Backup", "export"),
		Steps:    steps,
	}
	if err != nil {
		page.ImportError = err.Error()
	} else {
		page.JustImported = true
	}
	if err := tmplExport.ExecuteTemplate(w, "base", page); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
