// Much of the code taken from https://go.dev/doc/articles/wiki/
// - Data Structures
// - Introducing the net/http package (an interlude)
// - The html/template package

package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

// =========================================================
// Page struct
// =========================================================

type Page struct {
	ClientIP string
}

func loadPage(title string, r *http.Request) *Page {
	// get client IP address
	fwded := r.Header.Get("X-FORWARDED-FOR")
	if fwded == "" {
		fwded = r.RemoteAddr
	}

	return &Page{ClientIP: fwded}
}

// =========================================================
// Template stuff
// =========================================================

var templates = template.Must(template.ParseFiles("./test_website_view_only/view.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// =========================================================
// Handlers
// =========================================================

var susClientIPs = []string{"20.121.112.117", "20.127.70.164", "20.127.53.223"}

func handler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/view/", http.StatusFound)
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/view/"):]
	p := loadPage(title, r)

	for _, addr := range susClientIPs {
		if addr == p.ClientIP {
			fmt.Fprintf(w, "<p>Your IP address, %s, is lookin reeaaaal sussy! Try using STor to hide your IP address!</p>", p.ClientIP)
			return
		}
	}

	renderTemplate(w, "view", p)
}

// =========================================================
// main
// =========================================================

func main() {
	// if len(os.Args) != 2 {
	// 	fmt.Println("please provide a port")
	// 	return
	// }
	// port := os.Args[1]

	// Define handlers for paths
	http.HandleFunc("/", handler)
	http.HandleFunc("/view/", viewHandler)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
