// Command docs builds or serves the documentation website.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/docsite"
)

func main() {
	output := flag.String("out", "website/dist", "static output directory")
	address := flag.String("serve", "", "serve after building, e.g. 127.0.0.1:4173")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if err := docsite.Build(root, *output, core.Version); err != nil {
		log.Fatal(err)
	}
	absolute, err := filepath.Abs(*output)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Documentation built:", absolute)
	if *address != "" {
		fmt.Println("Local URL: http://" + *address)
		server := &http.Server{Addr: *address, Handler: http.FileServer(http.Dir(absolute)), ReadHeaderTimeout: 5e9}
		log.Fatal(server.ListenAndServe())
	}
}
