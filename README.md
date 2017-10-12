# Concurrency Made Easy

* [video](https://youtu.be/yKQOunhhf4A)
* [slides](https://dave.cheney.net/paste/concurrency-made-easy.pdf)

## Always release locks and semaphores in the reverse order to which you acquired them.

```
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

```

[In this example](https://play.golang.org/p/uksJ4-nnr0), simplified from a prior
version of gb-vendor, we’re attempting to fetch a set of dependencies from their
remote repositories, in parallel.

It turns out that there are several problems with this piece of code.

As a code reviewer my first point of concern is the interaction between this section:

```
defer func() {
	wg.Done()
	<-sem
}()
```

and this section:

```
wg.Wait()
close(sem)
```

My question for you is, `close(sem)` happens after `wg.Wait()` therefore it also
happens after `wg.Done()`, but not necessarily after `<-sem` — the close could occur before.

Could this cause a panic?

As it happens, no it cannot cause a panic.

Either `<-sem` happens before `close(sem)`, in which case it drains a value from
`sem` and then `sem` is marked closed, or `close(sem)` occurs first.

What does receiving from a closed channel return? **the zero value**

And when does it return? **immediately**

But you had to think about it to be sure. The logic is unnecessarily confusing.
If we simplify the defer statement and reorder the operations [to
get](https://play.golang.org/p/xmzIEzN04J)...

```
func restore(repos []string) error {
	errChan := make(chan error, 1)
	sem := make(chan int, 4) // four jobs at once
	var wg sync.WaitGroup
	wg.Add(len(repos))
	for _, repo := range repos {
		sem <- 1
		go func() {
			defer wg.Done()
			if err := fetch(repo); err != nil {
				errChan <- err
			}
			<-sem
		}()
	}
	wg.Wait()
	close(sem)
	close(errChan)
	return <-errChan
}
```

Now there is no question in which order the operations will occur.

In this program we add to the wait group, then push a value onto the `sem` channel, raising the level of the semaphore.
When each goroutine is done, the reverse occurs, we remove a value from the
`sem` channel, lowering the semaphore, and then defer calls `wg.Done` as the final operation,
to indicate that the goroutine is done.

So my suggestion to you is...

> Always release locks and semaphores in the reverse order to which you acquired them.

At best, mixing the order of acquire and release generates confusing code which is difficult to reason about.
At worst, mixing acquire and release leads to lock inversion and deadlocks.

### diff

```diff
@@ -29,13 +29,11 @@ func restore(repos []string) error {
 	for _, repo := range repos {
 		sem <- 1
 		go func() {
-			defer func() {
-				wg.Done()
-				<-sem
-			}()
+			defer wg.Done()
 			if err := fetch(repo); err != nil {
 				errChan <- err
 			}
+			<-sem
 		}()
 	}
 	wg.Wait()
```
