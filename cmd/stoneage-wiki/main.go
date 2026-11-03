// Command stoneage-wiki serves the embedded static wiki on its own, without
// the Web client backend that hosts it in the reference project.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/athlan20/stoneage/internal/gamewiki"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "HTTP listen address")
	cdn := flag.String("cdn", "", "HTTPS base URL for wiki images and map media (empty: no external images)")
	flag.Parse()

	wiki := gamewiki.NewHandler(*cdn)
	mux := http.NewServeMux()
	mux.Handle("/wiki", wiki)
	mux.Handle("/wiki/", wiki)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/wiki/", http.StatusFound)
	})

	log.Printf("wiki listening on http://%s/wiki/ (media from %s)", *listen, *cdn)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
