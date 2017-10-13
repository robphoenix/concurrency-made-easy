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

func restore(repos []string) error {
	errChan := make(chan error, 1)
	sem := make(chan int, 4) // four jobs at once
	var wg sync.WaitGroup
	wg.Add(len(repos))
	for _, repo := range repos {
		sem <- 1
		go func() {
			defer func() {
				wg.Done()
				<-sem
			}()
			if err := fetch(repo); err != nil {
				errChan <- err
			}
		}()
	}
	wg.Wait()
	close(sem)
	close(errChan)
	return <-errChan
}

func fetch(repo string) error {
	fmt.Printf("fetching repo = %+v\n", repo)
	return nil
}
