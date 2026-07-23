package main

import (
	"log"

	"github.com/sjzsdu/tongstock/internal/serverapp"
)

func main() {
	if err := serverapp.Run(); err != nil {
		log.Fatal(err)
	}
}
