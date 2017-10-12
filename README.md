# Concurrency Made Easy

* [video](https://youtu.be/yKQOunhhf4A)
* [slides](https://dave.cheney.net/paste/concurrency-made-easy.pdf)

## Let’s start with the go keyword.

Here’s a simple Web 2.0 Hello World program. Can anyone tell me what’s wrong with it?

```
func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, GopherCon SG")
	})
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	for {}
}
```

That’s right, `for{}` is an infinite loop.

`for{}` is going to block the main goroutine because it doesn’t do any IO, wait
on a lock, send or receive on a channel, or otherwise communicate with the
scheduler.  As the runtime is mostly cooperatively scheduled, this program is
going to spin fruitlessly on a single CPU, and may eventually end up
live-locked.  How could we fix this? Here’s one suggestion...

```
func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, GopherCon SG")
	})
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	for {
		runtime.Gosched()
	}
}
```

This is a common solution I see. It’s symptomatic of not understanding the
underlying problem.  Now, if you’re a little more experienced with go, you might
instead write something like this...

```
func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, GopherCon SG")
	})
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	select {}
}
```

An empty select statement will block forever. This is a useful property because
now we’re not spinning a whole CPU just to call `runtime.GoSched()`.  However,
as I said before, we’re only treating the symptom, not the cause.

I want to present to you another solution, one which has hopefully already
  occurred to you.

```
func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, GopherCon SG")
	})
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
```

Rather than run `ListenAndServe` in a goroutine, leaving us with the problem of
what to do with the main goroutine Simply run `ListenAndServe` on the main
goroutine itself.

So this is my first recommendation:

> If your goroutine cannot make progress until it gets the result from another,
  oftentimes it is simpler to just do the work yourself rather than to delegate
  it.

This often coincides with eliminating a lot of state tracking and channel
manipulation required to plumb a result back from a goroutine to its initiator.

## Always release locks and semaphores in the reverse order to which you acquired them.

[In this example](https://play.golang.org/p/uksJ4-nnr0), simplified from a prior
version of gb-vendor, we’re attempting to fetch a set of dependencies from their
remote repositories, in parallel.

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
happens after `wg.Done()`, but not necessarily after `<-sem` - the close could occur before.

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

Now there is no question in which order the operations will occur.

In this program we add to the wait group, then push a value onto the `sem` channel, raising the level of the semaphore.
When each goroutine is done, the reverse occurs, we remove a value from the
`sem` channel, lowering the semaphore, and then defer calls `wg.Done` as the final operation,
to indicate that the goroutine is done.

So my recommendation to you is...

> Always release locks and semaphores in the reverse order to which you acquired them.

At best, mixing the order of acquire and release generates confusing code which is difficult to reason about.
At worst, mixing acquire and release leads to lock inversion and deadlocks.

## Why `close(sem)`?

Now that we’ve rearranged the program a little, we can ask another question.
What is the reason for closing `sem`?  If a restaurant closes, it does not remove
anyone seated at that time, it’s just an indication that the restaurant is not
taking additional patrons.  Similarly, channels are not resources like files or
network sockets; the close signal does not free a channel, it just marks that
channel as no longer accepting new values.  In our example nothing is waiting in
a select or range loop for a close signal on `sem`, so we can remove the
`close(sem)` call.

> Channels aren’t resources like files or sockets, you don’t need to close them
  to free them.

Closing a channel is a signal to its receivers that it is no longer accepting
new data; nothing more, nothing less.

Closing a channel is not necessary to *"free"* a channel. You don’t need to close
a channel to *"clean up"* its resources.

## Speaking of semaphores, let’s look at how sem is used.

The role of `sem` is to make sure that at any one time, there is a cap on the
number of fetch operations running. In this example, `sem` has a capacity of
four.  But if you look closely, `sem` isn’t guaranteeing there are no more than
four fetch operations running, it’s guaranteeing that there are no more than
four goroutines running.

```
sem := make(chan int, 4) // four jobs at once

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
```

Assuming there are enough values in `repos`, each time through the loop we try
to push the number 1 onto the `sem` channel, then we fire off a fetch goroutine.
What happens when it’s the fifth time through the loop? Most likely we’ll have
four fetch goroutines running, or possibly those four goroutines won’t even have
been scheduled to run yet - remember that the scheduler doesn’t give any
guarantees if it will run a goroutine immediately, or schedule for later.  On
the fifth iteration the main loop is going to block trying to push a value onto
`sem`. Rather than spawning `len(repos)` goroutines which coordinate amongst
themselves for a semaphore, this loop will proceed at the rate that fetch
invocations finish.  While it doesn’t matter in this example - restore blocks
until all `repos` have been fetched - there are many situations where the
calling code may expect the function scheduling its work to complete quickly and
return, while the work occurs in the background.

The solution to this problem is to move `sem <- 1` inside the goroutine.

```
func restore(repos []string) error {
	errChan := make(chan error, 1)
	sem := make(chan int, 4) // four jobs at once
	var wg sync.WaitGroup
	wg.Add(len(repos))
	for _, repo := range repos {
		go func() {
			defer wg.Done()
			sem <- 1
			if err := fetch(repo); err != nil {
				errChan <- err
			}
			<-sem
		}()
	}
	wg.Wait()
	close(errChan)
	return <-errChan
}
```

```diff
@@ -15,9 +15,9 @@ func restore(repos []string) error {
         var wg sync.WaitGroup
         wg.Add(len(repos))
         for _, repo := range repos {
-                sem <- 1
                 go func() {
                         defer wg.Done()
+                        sem <- 1
                         if err := fetch(repo); err != nil {
                                 errChan <- err
                         }
```

Now all the fetch goroutines will be created immediately, and will negotiate a semaphore when they get scheduled by the runtime.

And this leads to my next recommendation:

> Acquire semaphores when you’re ready to use them.

Although goroutines are cheap to create and schedule, the resources they operate
on, files, sockets, bandwidth, and so on, are often scarcer. The pattern of
using a channel as a semaphore to limit work in progress is quite common.
However, to make sure that you don’t unduly block the code offloading work to a
goroutine, acquire a semaphore when you’re ready to use them, not when you
expect to use them.
