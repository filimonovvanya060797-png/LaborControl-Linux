package main

import "net/http"

func (h *controller) registerTab2(mux *http.ServeMux) {
	mux.HandleFunc("/api/tab2/state", h.wrap(h.tab2State))
	mux.HandleFunc("/api/tab2/import/fact", h.wrap(h.tab2ImportFact))
	mux.HandleFunc("/api/tab2/import/sz-rvd", h.wrap(h.tab2ImportSZRVD))
	mux.HandleFunc("/api/tab2/import/sap", h.wrap(h.tab2ImportSAP))
	mux.HandleFunc("/api/tab2/imports", h.wrap(h.tab2ImportHistory))
	mux.HandleFunc("/api/tab2/debug", h.wrap(h.tab2ExportDebug))
}