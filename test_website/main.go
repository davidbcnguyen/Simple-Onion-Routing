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
	"os"
)

// =========================================================
// Page struct
// =========================================================

type Page struct {
	Body     []byte
	ClientIP string
}

func loadPage() *Page {
	filename := "database.txt"
	body, err := os.ReadFile(filename)
	if err != nil {
		return &Page{}
	}
	return &Page{Body: body}
}

func (p *Page) save() error {
	filename := "database.txt"
	return os.WriteFile(filename, p.Body, 0600)
}

// =========================================================
// Template stuff
// =========================================================

// modify this if you add new html pages
var templates = template.Must(template.ParseFiles("./test_website/save.html", "./test_website/edit.html", "./test_website/view.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getIPAddressFromRequest(r *http.Request) string {
	fwded := r.Header.Get("X-FORWARDED-FOR")
	if fwded != "" {
		return fwded
	} else {
		return r.RemoteAddr
	}
}

var susClientIPs = []string{"20.121.112.117", "20.127.70.164", "20.127.53.223"}

func checkIfSusIPAddr(addr string) bool {
	for _, susAddr := range susClientIPs {
		if susAddr == addr {
			return true
		}
	}

	return false
}

// =========================================================
// Handlers
// =========================================================

func viewHandler(w http.ResponseWriter, r *http.Request) {
	p := loadPage()

	// get client IP address
	p.ClientIP = getIPAddressFromRequest(r)

	if checkIfSusIPAddr(p.ClientIP) {
		fmt.Fprintf(w, "<p>Your IP address, %s, is lookin reeaaaal sussy! Try using STor to hide your IP address!</p>", p.ClientIP)
		return
	}

	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
	p := loadPage()
	p.ClientIP = getIPAddressFromRequest(r)

	if checkIfSusIPAddr(p.ClientIP) {
		fmt.Fprintf(w, "<p>Your IP address, %s, is lookin reeaaaal sussy! Try using STor to hide your IP address!</p>", p.ClientIP)
		return
	}

	renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("body")
	p := &Page{Body: []byte(body), ClientIP: getIPAddressFromRequest(r)}

	if checkIfSusIPAddr(p.ClientIP) {
		fmt.Fprintf(w, "<p>Your IP address, %s, is lookin reeaaaal sussy! Try using STor to hide your IP address!</p>", p.ClientIP)
		return
	}

	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderTemplate(w, "save", p)
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
	http.HandleFunc("/", viewHandler)
	http.HandleFunc("/view/", viewHandler)
	http.HandleFunc("/edit/", editHandler)
	http.HandleFunc("/save/", saveHandler)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
