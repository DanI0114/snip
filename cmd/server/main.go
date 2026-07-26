package main

import (
	"fmt"
	"net/http"
)

func main() {
	//Router setup
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("./static"))                //variable to store my static files such as JS, Css, and html
	mux.Handle("./static/", http.StripPrefix("./static/", fs)) //give http a command to use this variable

	//Home Page
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})

	fmt.Println("Server running on http://localhost:8080")
	http.ListenAndServe(":8080", mux) //start a web server and direct it to port 8080 or the routring

}
