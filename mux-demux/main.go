package main

import (
	"fmt"
	"sync"
)

type Mux struct{}

// func (m *Mux) FanOutRequests(done <-chan bool, req []interface{}) chan interface{} {
// 	input := make(chan interface{})
// 	go func() {
// 		for r := range req {
// 			select {
// 			case <-done:
// 				return
// 			case input <- r:
// 			}
// 		}
// 	}()
// 	return input
// }

// func (m *Mux) ProcessRequests(done chan bool, req chan interface{}) chan interface{} {
// 	output := make(chan interface{})
// 	go func() {
// 		for r := range req {
// 			select {
// 			case <-done:
// 				return
// 			case output <- r:
// 			}
// 		}
// 		close(output)
// 	}()
// 	return output
// }

// func (m *Mux) FanInResponses(done chan bool, req []chan interface{}) chan interface{} {
// 	output := make([]interface{}, len(req))
// 	go func() {
// 		for r := range req {
// 			select {
// 			case <-done:
// 				return
// 			case output <- r:
// 			}
// 		}
// 		close(output)
// 	}()
// 	return output
// }

func (m *Mux) ProcessRequests(reqs []interface{}) {
	input := make(chan interface{}, len(reqs))
	var wg sync.WaitGroup
	for _, req := range reqs {
		wg.Add(1)
		go func(input <-chan interface{}) {
			defer wg.Done()
			select {
			case val, ok := <-input:
				if !ok {
					fmt.Println("Channel closed")
					return
				}
				fmt.Println(val)
			}
		}(input)
		input <- req
	}
	wg.Wait()
}

func (m *Mux) FanOutRequests(done chan bool, reqs []interface{}) chan interface{} {
	var wg sync.WaitGroup
	input := make(chan interface{})
	for _, req := range reqs {
		go func() {
			select {
			case <-done:
				close(input)
			case input <- req:
			}
		}()
	}
	wg.Wait()
	defer close(input)
	return input
}

func (m *Mux) ExecuteRequest(input chan interface{}) chan interface{} {
	output := make(chan interface{})
	process := func(val interface{}) interface{} {
		return val
	}
	defer close(output)
	go func() {
		select {
		case output <- process(<-input):

		}
	}()
	return output
}

func (m *Mux) MergeResponses(input chan interface{}, responses []chan interface{}) chan interface{} {
	var wg sync.WaitGroup
	merged := make(chan interface{})
	for _, res := range responses {
		wg.Add(1)
		go func(res chan interface{}) {
			for c := range res {
				select {
				case <-input:
					return
				case merged <- c:
				}
			}
		}(res)
	}

	go func() {
		wg.Done()
		close(merged)
	}()

	return merged
}

func main() {
	mux := &Mux{}
	mux.ProcessRequests([]interface{}{1, 2, 3})
}
