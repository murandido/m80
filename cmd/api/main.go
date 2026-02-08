package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("🚀 API M-80 rodando na porta 8080...")

	// Rota simples de teste
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("API Operante! ✈️"))
		if err != nil {
			panic(err)
		}
	})

	// Bloqueia a execução ouvindo a porta
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
