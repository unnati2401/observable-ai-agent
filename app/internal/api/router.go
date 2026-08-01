package api

import "net/http"

func RegisterRoutes() {
	http.HandleFunc("/hello", HelloHandler)
}
