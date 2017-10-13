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
	for i := range repos {
		go func(repo string) {
			defer wg.Done()
			sem <- 1
			if err := fetch(repo); err != nil {
				errChan <- err
			}
			<-sem
		}(repos[i])
	}
	wg.Wait()
	close(errChan)
	return <-errChan
}
