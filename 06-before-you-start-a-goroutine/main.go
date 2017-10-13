package main

import (
	"fmt"
	"sync"
)

func main() {
	repos := []string{
		"fatih/vim-go",
		"pkg/errors",
		"rakyll/gotest",
		"spf13/cobra",
		"golang/go",
	}
	restore(repos)
}

func fetch(repo string) error {
	fmt.Printf("fetching repo = %+v\n", repo)
	return nil
}

func restore(repos []string) error {
	errChan := make(chan error, 1)
	sem := make(chan int, 4) // four jobs at once
	var wg sync.WaitGroup
	wg.Add(len(repos))
	for _, repo := range repos {
		go worker(repo, sem, &wg, errChan)
	}
	wg.Wait()
	close(errChan)
	return <-errChan
}

func worker(repo string, sem chan int, wg *sync.WaitGroup, errChan chan error) {
	defer wg.Done()
	sem <- 1
	if err := fetch(repo); err != nil {
		select {
		case errChan <- err:
			// we're the first worker to fail
		default:
			// some other failure has already happened
		}
	}
	<-sem
}
