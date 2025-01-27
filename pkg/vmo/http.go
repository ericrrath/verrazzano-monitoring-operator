// Copyright (C) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package vmo

import (
	"net/http"
	"path"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// StartHTTPServer runs an embedded HTTP server for any VMO handlers as a "resilient" goroutine meaning it runs in
// the background and will be restarted if it dies.
func StartHTTPServer(controller *Controller, certdir string, port string) {
	setupHandlers(controller)
	go wait.Until(func() {
		controller.log.Oncef("Starting HTTP server")
		err := http.ListenAndServeTLS(":"+port, path.Join(certdir, "tls.crt"), path.Join(certdir, "tls.key"), nil)
		if err != nil {
			controller.log.Errorf("Failed to start HTTP server for VMI: %v", err)
		}
	}, time.Second*3, wait.NeverStop)
}

func setupHandlers(controller *Controller) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if controller.IsHealthy() {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("ok")); err != nil {
				controller.log.Errorf("Failed writing healthcheck response: %v", err)
			}
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}
