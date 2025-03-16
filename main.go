package main

import (
	"fmt"

	"github.com/sourcegraph/conc/pool"
)

func main() {

	stream := make(chan string)

	go processEvents(stream)
	runMQTT(stream)

}

func processEvents(stream chan string) {
	p := pool.New().WithMaxGoroutines(10)
	for elem := range stream {
		elem := elem
		p.Go(func() {
			fmt.Println(elem)
		})
	}
	p.Wait()

}
