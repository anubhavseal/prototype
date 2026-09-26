package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	serve := http.NewServeMux()
	serve.HandleFunc("/hello", LogRequest(AddDate(HelloWorld)))
	http.ListenAndServe(":8080", serve)
}

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	fmt.Println("All headers:", r.Header)
	fmt.Println("Hello header value:", r.Header.Get("hello"))
	w.Write([]byte("hello world"))
}

func AddDate(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add("time", time.Now().Format("2025-02-01"))
		handler(w, r)
	}
}

func LogRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Request", r)
		handler(w, r)
	}
}
